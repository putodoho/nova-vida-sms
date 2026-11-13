// Package access manages staff records, user accounts and roles.
package access

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	manage := auth.Require(auth.UsersManage)

	// Any signed-in user may list staff (teacher pickers, contacts).
	r.Method(http.MethodGet, "/staff", httpx.Handler(h.listStaff))
	r.With(manage).Method(http.MethodPost, "/staff", httpx.Handler(h.createStaff))
	r.With(manage).Method(http.MethodPut, "/staff/{id}", httpx.Handler(h.updateStaff))

	r.With(manage).Method(http.MethodGet, "/users", httpx.Handler(h.listUsers))
	r.With(manage).Method(http.MethodPost, "/users", httpx.Handler(h.createUser))
	r.With(manage).Method(http.MethodPut, "/users/{id}", httpx.Handler(h.updateUser))
	r.With(manage).Method(http.MethodPost, "/users/{id}/reset-password", httpx.Handler(h.resetPassword))

	r.With(manage).Method(http.MethodGet, "/roles", httpx.Handler(h.listRoles))
	r.With(manage).Method(http.MethodPost, "/roles", httpx.Handler(h.createRole))
	r.With(manage).Method(http.MethodPut, "/roles/{id}", httpx.Handler(h.updateRole))
	r.With(manage).Method(http.MethodDelete, "/roles/{id}", httpx.Handler(h.deleteRole))
}

// ---------------------------------------------------------------------------
// Staff
// ---------------------------------------------------------------------------

type Staff struct {
	ID       uuid.UUID  `json:"id" db:"id"`
	FullName string     `json:"full_name" db:"full_name"`
	Function string     `json:"function" db:"function"`
	Phone    string     `json:"phone" db:"phone"`
	Email    string     `json:"email" db:"email"`
	Active   bool       `json:"active" db:"active"`
	UserID   *uuid.UUID `json:"user_id" db:"user_id"` // linked system account, if any
}

const staffSelect = `
	SELECT s.id, s.full_name, s.function, s.phone, s.email, s.active,
	       (SELECT u.id FROM app_user u WHERE u.staff_id = s.id LIMIT 1) AS user_id
	FROM staff s`

func (h *Handler) listStaff(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := h.Pool.Query(r.Context(), staffSelect+`
		WHERE ($1 = '' OR s.full_name ILIKE '%' || $1 || '%' OR s.function ILIKE '%' || $1 || '%')
		  AND ($2 OR s.active)
		ORDER BY s.active DESC, s.full_name`, q, r.URL.Query().Get("include_inactive") == "true")
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Staff])
	if err != nil {
		return err
	}
	if items == nil {
		items = []Staff{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Staff]{Items: items, Total: len(items)})
	return nil
}

func loadStaff(ctx context.Context, q db.Querier, id uuid.UUID) (Staff, error) {
	rows, err := q.Query(ctx, staffSelect+` WHERE s.id = $1`, id)
	if err != nil {
		return Staff{}, err
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Staff])
	if errors.Is(err, pgx.ErrNoRows) {
		return s, httpx.NotFound()
	}
	return s, err
}

type staffRequest struct {
	FullName string `json:"full_name"`
	Function string `json:"function"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Active   *bool  `json:"active"`
}

func (req *staffRequest) validate() error {
	f := httpx.Fields{}
	req.FullName = strings.TrimSpace(req.FullName)
	req.Email = strings.TrimSpace(req.Email)
	f.Add(req.FullName == "", "full_name", "Obrigatório.")
	f.Add(req.Email != "" && !strings.Contains(req.Email, "@"), "email", "Email inválido.")
	return f.Err()
}

func (h *Handler) createStaff(w http.ResponseWriter, r *http.Request) error {
	var req staffRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var out Staff
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO staff (full_name, function, phone, email, active) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			req.FullName, strings.TrimSpace(req.Function), strings.TrimSpace(req.Phone), req.Email, req.Active == nil || *req.Active).Scan(&id); err != nil {
			return err
		}
		var err error
		if out, err = loadStaff(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "create", Table: "staff", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateStaff(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req staffRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var out Staff
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadStaff(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE staff SET full_name=$2, function=$3, phone=$4, email=$5, active=$6 WHERE id=$1`,
			id, req.FullName, strings.TrimSpace(req.Function), strings.TrimSpace(req.Phone), req.Email, req.Active == nil || *req.Active); err != nil {
			return err
		}
		if out, err = loadStaff(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "staff", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

type User struct {
	ID          uuid.UUID   `json:"id" db:"id"`
	StaffID     *uuid.UUID  `json:"staff_id" db:"staff_id"`
	Login       string      `json:"login" db:"login"`
	DisplayName string      `json:"display_name" db:"display_name"`
	Active      bool        `json:"active" db:"active"`
	LastLoginAt *time.Time  `json:"last_login_at" db:"last_login_at"`
	RoleIDs     []uuid.UUID `json:"role_ids" db:"role_ids"`
	RoleCodes   []string    `json:"role_codes" db:"role_codes"`
	EntityIDs   []uuid.UUID `json:"entity_ids" db:"entity_ids"`
}

const userSelect = `
	SELECT u.id, u.staff_id, u.login::text AS login, u.display_name, u.active, u.last_login_at,
	       COALESCE((SELECT array_agg(ur.role_id ORDER BY r.code) FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = u.id), '{}') AS role_ids,
	       COALESCE((SELECT array_agg(r.code ORDER BY r.code) FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = u.id), '{}') AS role_codes,
	       COALESCE((SELECT array_agg(ue.entity_id ORDER BY ue.entity_id) FROM user_entity ue WHERE ue.user_id = u.id), '{}') AS entity_ids
	FROM app_user u`

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), userSelect+` ORDER BY u.active DESC, u.display_name`)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[User])
	if err != nil {
		return err
	}
	if items == nil {
		items = []User{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[User]{Items: items, Total: len(items)})
	return nil
}

func loadUser(ctx context.Context, q db.Querier, id uuid.UUID) (User, error) {
	rows, err := q.Query(ctx, userSelect+` WHERE u.id = $1`, id)
	if err != nil {
		return User{}, err
	}
	u, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
	if errors.Is(err, pgx.ErrNoRows) {
		return u, httpx.NotFound()
	}
	return u, err
}

type userRequest struct {
	Login       string      `json:"login"`
	DisplayName string      `json:"display_name"`
	StaffID     *uuid.UUID  `json:"staff_id"`
	Password    string      `json:"password"` // create only
	Active      *bool       `json:"active"`
	RoleIDs     []uuid.UUID `json:"role_ids"`
	EntityIDs   []uuid.UUID `json:"entity_ids"`
}

// validate checks fields and that the acting user only grants entities they
// can access themselves.
func (req *userRequest) validate(ctx context.Context, q db.Querier, creating bool) error {
	f := httpx.Fields{}
	req.Login = strings.ToLower(strings.TrimSpace(req.Login))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	f.Add(creating && len(req.Login) < 3, "login", "Obrigatório (email ou telefone).")
	f.Add(req.DisplayName == "", "display_name", "Obrigatório.")
	f.Add(creating && len(req.Password) < auth.MinPasswordLen, "password", "Mínimo de 8 caracteres.")
	f.Add(len(req.RoleIDs) == 0, "role_ids", "Atribua pelo menos uma função.")
	f.Add(len(req.EntityIDs) == 0, "entity_ids", "Atribua pelo menos uma empresa.")

	actor := auth.CurrentUser(ctx)
	for _, e := range req.EntityIDs {
		f.Add(!actor.CanAccessEntity(e), "entity_ids", "Só pode atribuir empresas a que tem acesso.")
	}
	var roles, staffOK int
	if err := q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM role WHERE id = ANY($1))::int,
		       CASE WHEN $2::uuid IS NULL THEN 1 ELSE (SELECT count(*) FROM staff WHERE id = $2)::int END`,
		req.RoleIDs, req.StaffID).Scan(&roles, &staffOK); err != nil {
		return err
	}
	f.Add(roles != len(dedupe(req.RoleIDs)), "role_ids", "Função inválida.")
	f.Add(staffOK != 1, "staff_id", "Funcionário inválido.")
	return f.Err()
}

func dedupe(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(out)
}

func setUserLinks(ctx context.Context, tx pgx.Tx, userID uuid.UUID, roleIDs, entityIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_role WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_role (user_id, role_id) SELECT $1, unnest($2::uuid[])`, userID, dedupe(roleIDs)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_entity WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO user_entity (user_id, entity_id) SELECT $1, unnest($2::uuid[])`, userID, dedupe(entityIDs))
	return err
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) error {
	var req userRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out User
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if err := req.validate(r.Context(), tx, true); err != nil {
			return err
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			return err
		}
		var id uuid.UUID
		err = tx.QueryRow(r.Context(), `
			INSERT INTO app_user (login, display_name, staff_id, password_hash, active) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			req.Login, req.DisplayName, req.StaffID, hash, req.Active == nil || *req.Active).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"login": "Já existe um utilizador com este login."})
		}
		if err != nil {
			return err
		}
		if err := setUserLinks(r.Context(), tx, id, req.RoleIDs, req.EntityIDs); err != nil {
			return err
		}
		if out, err = loadUser(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "create", Table: "app_user", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req userRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	actor := auth.CurrentUser(r.Context())
	active := req.Active == nil || *req.Active
	var out User
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadUser(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := req.validate(r.Context(), tx, false); err != nil {
			return err
		}
		if id == actor.ID && !active {
			return httpx.Conflict("self_deactivate", "Não pode desactivar a sua própria conta.")
		}
		// Entities the actor cannot see stay as they were.
		for _, e := range before.EntityIDs {
			if !actor.CanAccessEntity(e) && !slices.Contains(req.EntityIDs, e) {
				req.EntityIDs = append(req.EntityIDs, e)
			}
		}
		if _, err := tx.Exec(r.Context(), `UPDATE app_user SET display_name=$2, staff_id=$3, active=$4 WHERE id=$1`,
			id, req.DisplayName, req.StaffID, active); err != nil {
			return err
		}
		if err := setUserLinks(r.Context(), tx, id, req.RoleIDs, req.EntityIDs); err != nil {
			return err
		}
		if err := ensureActiveAdmin(r.Context(), tx); err != nil {
			return err
		}
		if !active {
			if _, err := tx.Exec(r.Context(), `DELETE FROM session WHERE user_id = $1`, id); err != nil {
				return err
			}
		}
		if out, err = loadUser(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "app_user", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// ensureActiveAdmin refuses changes that leave no active administrator.
func ensureActiveAdmin(ctx context.Context, tx pgx.Tx) error {
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM app_user u
		JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
		WHERE u.active AND r.code = 'admin'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return httpx.Conflict("last_admin", "Tem de existir pelo menos um administrador activo.")
	}
	return nil
}

type resetRequest struct {
	Password string `json:"password"`
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req resetRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if len(req.Password) < auth.MinPasswordLen {
		return httpx.Validation(httpx.Fields{"password": "Mínimo de 8 caracteres."})
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE app_user SET password_hash = $2 WHERE id = $1`, id, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound()
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM session WHERE user_id = $1`, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "password_reset", Table: "app_user", RecordID: id.String()})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

type Role struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Code        string    `json:"code" db:"code"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	System      bool      `json:"system" db:"system"`
	Permissions []string  `json:"permissions" db:"permissions"`
	Users       int       `json:"users" db:"users"`
}

const roleSelect = `
	SELECT r.id, r.code, r.name, r.description, r.system,
	       COALESCE((SELECT array_agg(rp.permission ORDER BY rp.permission) FROM role_permission rp WHERE rp.role_id = r.id), '{}') AS permissions,
	       (SELECT count(*) FROM user_role ur WHERE ur.role_id = r.id)::int AS users
	FROM role r`

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), roleSelect+` ORDER BY r.system DESC, r.name`)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Role])
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Role]{Items: items, Total: len(items)})
	return nil
}

func loadRole(ctx context.Context, q db.Querier, id uuid.UUID) (Role, error) {
	rows, err := q.Query(ctx, roleSelect+` WHERE r.id = $1`, id)
	if err != nil {
		return Role{}, err
	}
	role, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Role])
	if errors.Is(err, pgx.ErrNoRows) {
		return role, httpx.NotFound()
	}
	return role, err
}

type roleRequest struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (req *roleRequest) validate(creating bool) error {
	f := httpx.Fields{}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	f.Add(creating && (len(req.Code) < 2 || strings.ContainsAny(req.Code, " /")), "code", "Código inválido (ex.: tesouraria).")
	f.Add(req.Name == "", "name", "Obrigatório.")
	for _, p := range req.Permissions {
		f.Add(!auth.IsValid(auth.Permission(p)), "permissions", "Permissão desconhecida: "+p)
	}
	slices.Sort(req.Permissions)
	req.Permissions = slices.Compact(req.Permissions)
	return f.Err()
}

func setRolePermissions(ctx context.Context, tx pgx.Tx, roleID uuid.UUID, perms []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permission WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO role_permission (role_id, permission) SELECT $1, unnest($2::text[])`, roleID, perms)
	return err
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) error {
	var req roleRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(true); err != nil {
		return err
	}
	var out Role
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(r.Context(), `INSERT INTO role (code, name, description) VALUES ($1, $2, $3) RETURNING id`,
			req.Code, req.Name, strings.TrimSpace(req.Description)).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"code": "Já existe uma função com este código."})
		}
		if err != nil {
			return err
		}
		if err := setRolePermissions(r.Context(), tx, id, req.Permissions); err != nil {
			return err
		}
		if out, err = loadRole(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "create", Table: "role", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req roleRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(false); err != nil {
		return err
	}
	var out Role
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadRole(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if before.Code == "admin" && len(req.Permissions) != len(auth.All) {
			return httpx.Conflict("admin_role_fixed", "A função Administrador tem sempre todas as permissões.")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE role SET name=$2, description=$3 WHERE id=$1`,
			id, req.Name, strings.TrimSpace(req.Description)); err != nil {
			return err
		}
		if err := setRolePermissions(r.Context(), tx, id, req.Permissions); err != nil {
			return err
		}
		if out, err = loadRole(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "role", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h *Handler) deleteRole(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadRole(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if before.System {
			return httpx.Conflict("system_role", "As funções do sistema não podem ser apagadas.")
		}
		if before.Users > 0 {
			return httpx.Conflict("role_in_use", "Retire esta função dos utilizadores antes de a apagar.")
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM role WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "delete", Table: "role", RecordID: id.String(), Before: before})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
