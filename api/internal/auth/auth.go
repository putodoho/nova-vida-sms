// Package auth implements login sessions, the current-user context, permission
// checks and the entity scope (X-Entity-ID).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

const CookieName = "nv_session"

// User is the authenticated user attached to the request context.
type User struct {
	ID          uuid.UUID    `json:"id"`
	Login       string       `json:"login"`
	DisplayName string       `json:"display_name"`
	Roles       []string     `json:"roles"`
	Permissions []Permission `json:"permissions"`
	EntityIDs   []uuid.UUID  `json:"entity_ids"`
	SessionID   uuid.UUID    `json:"-"`
	perms       map[Permission]bool
}

// Can reports whether the user holds permission p.
func (u *User) Can(p Permission) bool { return u != nil && u.perms[p] }

// CanAccessEntity reports whether the user may act on entity id.
func (u *User) CanAccessEntity(id uuid.UUID) bool {
	for _, e := range u.EntityIDs {
		if e == id {
			return true
		}
	}
	return false
}

type ctxKey int

const (
	userKey ctxKey = iota
	entityKey
)

// CurrentUser returns the authenticated user, or nil.
func CurrentUser(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// WithUser returns a context carrying u (used by tests and background jobs).
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// Scope is the entity scope of a request: one entity (writes and scoped
// reads) or all entities the user can access (aggregate reads).
type Scope struct {
	EntityID  uuid.UUID // zero when All
	All       bool
	EntityIDs []uuid.UUID // the entities this request may read
}

// EntityScope returns the request's entity scope.
func EntityScope(ctx context.Context) Scope {
	s, _ := ctx.Value(entityKey).(Scope)
	return s
}

// Service manages sessions.
type Service struct {
	Pool         *pgxpool.Pool
	TTL          time.Duration
	CookieSecure bool
	Now          func() time.Time
}

var ErrInvalidCredentials = errors.New("invalid credentials")

// Login checks credentials and creates a session; it returns the raw token for
// the cookie. Unknown users and wrong passwords return the same error.
func (s *Service) Login(ctx context.Context, login, password, ip, userAgent string) (string, uuid.UUID, error) {
	var userID uuid.UUID
	var hash string
	var active bool
	err := s.Pool.QueryRow(ctx,
		`SELECT id, password_hash, active FROM app_user WHERE login = $1`, strings.TrimSpace(login)).
		Scan(&userID, &hash, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		// Spend similar time as a real check to avoid user enumeration by timing.
		_, _ = CheckPassword(dummyHash, password)
		return "", uuid.Nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", uuid.Nil, err
	}
	ok, err := CheckPassword(hash, password)
	if err != nil {
		return "", uuid.Nil, err
	}
	if !ok || !active {
		return "", uuid.Nil, ErrInvalidCredentials
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", uuid.Nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := s.Pool.Exec(ctx,
		`INSERT INTO session (token_hash, user_id, expires_at, ip, user_agent) VALUES ($1, $2, $3, $4, $5)`,
		hashToken(token), userID, s.now().Add(s.TTL), ip, truncate(userAgent, 300)); err != nil {
		return "", uuid.Nil, err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE app_user SET last_login_at = now() WHERE id = $1`, userID); err != nil {
		return "", uuid.Nil, err
	}
	return token, userID, nil
}

// Logout deletes the session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM session WHERE id = $1`, sessionID)
	return err
}

// LogoutAll deletes every session of a user (password change, deactivation).
func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM session WHERE user_id = $1`, userID)
	return err
}

// SetCookie writes the session cookie.
func (s *Service) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/",
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(s.TTL.Seconds()),
	})
}

// ClearCookie removes the session cookie.
func (s *Service) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// Authenticate is middleware that loads the user from the session cookie and
// slides the session expiry. Requests without a valid session get 401.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			httpx.Fail(w, r, httpx.Unauthorized())
			return
		}
		u, err := s.UserFromToken(r.Context(), c.Value)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				s.ClearCookie(w)
				httpx.Fail(w, r, httpx.Unauthorized())
				return
			}
			httpx.Fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// UserFromToken loads the user of a valid session and slides its expiry.
// It returns pgx.ErrNoRows when the session is unknown, expired or the user inactive.
func (s *Service) UserFromToken(ctx context.Context, token string) (*User, error) {
	u := &User{perms: map[Permission]bool{}}
	err := s.Pool.QueryRow(ctx, `
		UPDATE session SET expires_at = $2, last_seen = now()
		FROM app_user
		WHERE session.token_hash = $1 AND session.expires_at > now()
		  AND app_user.id = session.user_id AND app_user.active
		RETURNING session.id, app_user.id, app_user.login, app_user.display_name`,
		hashToken(token), s.now().Add(s.TTL)).
		Scan(&u.SessionID, &u.ID, &u.Login, &u.DisplayName)
	if err != nil {
		return nil, err
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT r.code, rp.permission
		FROM user_role ur
		JOIN role r ON r.id = ur.role_id
		LEFT JOIN role_permission rp ON rp.role_id = r.id
		WHERE ur.user_id = $1`, u.ID)
	if err != nil {
		return nil, err
	}
	roles := map[string]bool{}
	for rows.Next() {
		var role string
		var perm *string
		if err := rows.Scan(&role, &perm); err != nil {
			rows.Close()
			return nil, err
		}
		if !roles[role] {
			roles[role] = true
			u.Roles = append(u.Roles, role)
		}
		if perm != nil && !u.perms[Permission(*perm)] {
			u.perms[Permission(*perm)] = true
			u.Permissions = append(u.Permissions, Permission(*perm))
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	erows, err := s.Pool.Query(ctx, `SELECT entity_id FROM user_entity WHERE user_id = $1 ORDER BY entity_id`, u.ID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(erows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	u.EntityIDs = ids
	if u.Roles == nil {
		u.Roles = []string{}
	}
	if u.Permissions == nil {
		u.Permissions = []Permission{}
	}
	return u, nil
}

// Require is middleware that rejects users lacking permission p.
func Require(p Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !CurrentUser(r.Context()).Can(p) {
				httpx.Fail(w, r, httpx.Forbidden())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// EntityHeader is the request header carrying the active entity.
const EntityHeader = "X-Entity-ID"

// ScopeEntity is middleware that resolves X-Entity-ID. A UUID must be an
// entity the user can access. "all" (or a missing header) is accepted only on
// safe methods and scopes reads to every entity the user can access; writes
// always need a single entity.
func ScopeEntity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := CurrentUser(r.Context())
		raw := strings.TrimSpace(r.Header.Get(EntityHeader))
		var s Scope
		switch raw {
		case "", "all":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				httpx.Fail(w, r, httpx.BadRequest("Seleccione uma empresa (cabeçalho X-Entity-ID)."))
				return
			}
			s = Scope{All: true, EntityIDs: u.EntityIDs}
		default:
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.Fail(w, r, httpx.BadRequest("X-Entity-ID inválido."))
				return
			}
			if !u.CanAccessEntity(id) {
				httpx.Fail(w, r, httpx.Forbidden())
				return
			}
			s = Scope{EntityID: id, EntityIDs: []uuid.UUID{id}}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), entityKey, s)))
	})
}

// WithScope returns a context carrying s (tests and background jobs).
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, entityKey, s)
}

// ClientIP returns the caller's IP, trusting X-Forwarded-For from the reverse proxy.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// dummyHash is checked when the login does not exist, so both paths cost the same.
var dummyHash = func() string {
	h, _ := HashPassword("not-a-real-password")
	return h
}()

// SingleEntity returns the one entity selected for this request, or a 400 when
// the request is scoped to all entities. Use it in every write handler.
func SingleEntity(ctx context.Context) (uuid.UUID, error) {
	s := EntityScope(ctx)
	if s.All || s.EntityID == uuid.Nil {
		return uuid.Nil, httpx.BadRequest("Seleccione uma empresa (cabeçalho X-Entity-ID).")
	}
	return s.EntityID, nil
}
