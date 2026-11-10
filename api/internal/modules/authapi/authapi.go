// Package authapi exposes login, logout, current user and password change.
package authapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

type Handler struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

// PublicRoutes need no session.
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Method(http.MethodPost, "/auth/login", httpx.Handler(h.login))
}

// Routes need a session.
func (h *Handler) Routes(r chi.Router) {
	r.Method(http.MethodPost, "/auth/logout", httpx.Handler(h.logout))
	r.Method(http.MethodGet, "/auth/me", httpx.Handler(h.me))
	r.Method(http.MethodPost, "/auth/password", httpx.Handler(h.changePassword))
	r.Method(http.MethodGet, "/permissions", httpx.Handler(h.permissions))
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	fields := map[string]string{}
	if strings.TrimSpace(req.Login) == "" {
		fields["login"] = "Obrigatório."
	}
	if req.Password == "" {
		fields["password"] = "Obrigatório."
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}

	token, userID, err := h.Auth.Login(r.Context(), req.Login, req.Password, auth.ClientIP(r), r.UserAgent())
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return &httpx.Error{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "Utilizador ou palavra-passe incorrectos."}
	}
	if err != nil {
		return err
	}
	ctx := auth.WithUser(r.Context(), &auth.User{ID: userID})
	if err := audit.Write(ctx, h.Pool, audit.Entry{Action: "login", Table: "app_user", RecordID: userID.String()}); err != nil {
		return err
	}
	h.Auth.SetCookie(w, token)

	me, err := h.loadMe(r.Context(), token)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, me)
	return nil
}

// loadMe builds the /auth/me payload for a fresh token (login response).
func (h *Handler) loadMe(ctx context.Context, token string) (*meResponse, error) {
	u, err := h.Auth.UserFromToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return h.buildMe(ctx, u)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	u := auth.CurrentUser(r.Context())
	if err := h.Auth.Logout(r.Context(), u.SessionID); err != nil {
		return err
	}
	h.Auth.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type entityRef struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	ShortName string    `json:"short_name"`
	LegalName string    `json:"legal_name"`
}

type meResponse struct {
	User     *auth.User  `json:"user"`
	Entities []entityRef `json:"entities"`
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	resp, err := h.buildMe(r.Context(), auth.CurrentUser(r.Context()))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, resp)
	return nil
}

func (h *Handler) buildMe(ctx context.Context, u *auth.User) (*meResponse, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT e.id, e.code, e.short_name, e.legal_name
		FROM entity e JOIN user_entity ue ON ue.entity_id = e.id
		WHERE ue.user_id = $1
		ORDER BY e.code DESC`, u.ID) // 'primary' before 'preschool'
	if err != nil {
		return nil, err
	}
	ents, err := pgx.CollectRows(rows, pgx.RowToStructByPos[entityRef])
	if err != nil {
		return nil, err
	}
	if ents == nil {
		ents = []entityRef{}
	}
	return &meResponse{User: u, Entities: ents}, nil
}

type passwordRequest struct {
	Current string `json:"current_password"`
	New     string `json:"new_password"`
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) error {
	var req passwordRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if len(req.New) < auth.MinPasswordLen {
		return httpx.Validation(map[string]string{"new_password": "Mínimo de 8 caracteres."})
	}
	u := auth.CurrentUser(r.Context())
	var hash string
	if err := h.Pool.QueryRow(r.Context(), `SELECT password_hash FROM app_user WHERE id = $1`, u.ID).Scan(&hash); err != nil {
		return err
	}
	ok, err := auth.CheckPassword(hash, req.Current)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Validation(map[string]string{"current_password": "Palavra-passe actual incorrecta."})
	}
	newHash, err := auth.HashPassword(req.New)
	if err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `UPDATE app_user SET password_hash = $2 WHERE id = $1`, u.ID, newHash); err != nil {
			return err
		}
		// Keep only the current session.
		if _, err := tx.Exec(r.Context(), `DELETE FROM session WHERE user_id = $1 AND id <> $2`, u.ID, u.SessionID); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "password_change", Table: "app_user", RecordID: u.ID.String()})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) permissions(w http.ResponseWriter, _ *http.Request) error {
	httpx.JSON(w, http.StatusOK, auth.All)
	return nil
}
