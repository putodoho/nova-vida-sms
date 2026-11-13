// Package settings manages school years, levels and penalty rules, classes,
// the fee catalogue and discount types.
package settings

import (
	"context"
	"errors"
	"net/http"
	"sort"
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

// GlobalRoutes are not entity-scoped (school years are shared by both entities).
func (h *Handler) GlobalRoutes(r chi.Router) {
	r.Method(http.MethodGet, "/school-years", httpx.Handler(h.listYears))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPost, "/school-years", httpx.Handler(h.createYear))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPut, "/school-years/{id}", httpx.Handler(h.updateYear))
}

// ScopedRoutes need X-Entity-ID.
func (h *Handler) ScopedRoutes(r chi.Router) {
	manage := auth.Require(auth.SettingsManage)

	r.Method(http.MethodGet, "/levels", httpx.Handler(h.listLevels))
	r.With(manage).Method(http.MethodPut, "/levels/{id}", httpx.Handler(h.updateLevel))

	r.Method(http.MethodGet, "/classes", httpx.Handler(h.listClasses))
	r.With(manage).Method(http.MethodPost, "/classes", httpx.Handler(h.createClass))
	r.With(manage).Method(http.MethodPut, "/classes/{id}", httpx.Handler(h.updateClass))
	r.With(manage).Method(http.MethodDelete, "/classes/{id}", httpx.Handler(h.deleteClass))

	r.Method(http.MethodGet, "/fee-items", httpx.Handler(h.listFees))
	r.With(manage).Method(http.MethodPost, "/fee-items", httpx.Handler(h.createFee))
	r.With(manage).Method(http.MethodPut, "/fee-items/{id}", httpx.Handler(h.updateFee))
	r.With(manage).Method(http.MethodPost, "/fee-items/copy-from-year", httpx.Handler(h.copyFees))

	r.Method(http.MethodGet, "/discount-types", httpx.Handler(h.listDiscountTypes))
	r.With(manage).Method(http.MethodPost, "/discount-types", httpx.Handler(h.createDiscountType))
	r.With(manage).Method(http.MethodPut, "/discount-types/{id}", httpx.Handler(h.updateDiscountType))
}

// ---------------------------------------------------------------------------
// School years
// ---------------------------------------------------------------------------

type SchoolYear struct {
	ID            uuid.UUID `json:"id" db:"id"`
	Year          int       `json:"year" db:"year"`
	StartsOn      string    `json:"starts_on" db:"starts_on"`
	EndsOn        string    `json:"ends_on" db:"ends_on"`
	BillingMonths []int32   `json:"billing_months" db:"billing_months"`
	DueDay        int       `json:"due_day" db:"due_day"`
	Status        string    `json:"status" db:"status"`
}

const yearCols = `id, year, starts_on::text AS starts_on, ends_on::text AS ends_on, billing_months, due_day, status`

func (h *Handler) listYears(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), `SELECT `+yearCols+` FROM school_year ORDER BY year DESC`)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[SchoolYear])
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[SchoolYear]{Items: nonNil(items), Total: len(items)})
	return nil
}

type yearRequest struct {
	Year          int     `json:"year"`
	StartsOn      string  `json:"starts_on"`
	EndsOn        string  `json:"ends_on"`
	BillingMonths []int32 `json:"billing_months"`
	DueDay        int     `json:"due_day"`
	Status        string  `json:"status"`
}

func (req *yearRequest) validate() error {
	f := httpx.Fields{}
	f.Add(req.Year < 2020 || req.Year > 2100, "year", "Ano inválido.")
	start, err1 := time.Parse(time.DateOnly, req.StartsOn)
	end, err2 := time.Parse(time.DateOnly, req.EndsOn)
	f.Add(err1 != nil, "starts_on", "Data inválida (AAAA-MM-DD).")
	f.Add(err2 != nil, "ends_on", "Data inválida (AAAA-MM-DD).")
	f.Add(err1 == nil && err2 == nil && !end.After(start), "ends_on", "Tem de ser depois da data de início.")
	f.Add(len(req.BillingMonths) == 0 || len(req.BillingMonths) > 12, "billing_months", "Indique entre 1 e 12 meses.")
	seen := map[int32]bool{}
	for _, m := range req.BillingMonths {
		f.Add(m < 1 || m > 12 || seen[m], "billing_months", "Meses inválidos ou repetidos.")
		seen[m] = true
	}
	sort.Slice(req.BillingMonths, func(i, j int) bool { return req.BillingMonths[i] < req.BillingMonths[j] })
	f.Add(req.DueDay < 1 || req.DueDay > 28, "due_day", "Dia entre 1 e 28.")
	switch req.Status {
	case "planning", "open", "closed":
	default:
		f.Add(true, "status", "Estado inválido.")
	}
	return f.Err()
}

func (h *Handler) loadYear(ctx context.Context, q db.Querier, id uuid.UUID) (SchoolYear, error) {
	rows, err := q.Query(ctx, `SELECT `+yearCols+` FROM school_year WHERE id = $1`, id)
	if err != nil {
		return SchoolYear{}, err
	}
	y, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[SchoolYear])
	if errors.Is(err, pgx.ErrNoRows) {
		return y, httpx.NotFound()
	}
	return y, err
}

func (h *Handler) createYear(w http.ResponseWriter, r *http.Request) error {
	var req yearRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var out SchoolYear
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(r.Context(), `
			INSERT INTO school_year (year, starts_on, ends_on, billing_months, due_day, status)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			req.Year, req.StartsOn, req.EndsOn, req.BillingMonths, req.DueDay, req.Status).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"year": "Este ano lectivo já existe."})
		}
		if err != nil {
			return err
		}
		if out, err = h.loadYear(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "create", Table: "school_year", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateYear(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req yearRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var out SchoolYear
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := h.loadYear(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if before.Year != req.Year {
			return httpx.Validation(httpx.Fields{"year": "O ano não pode ser alterado."})
		}
		// Billing calendar is frozen once charges exist for the year.
		var charged bool
		if err := tx.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM charge c JOIN enrolment e ON e.id = c.enrolment_id WHERE e.school_year_id = $1)`, id).Scan(&charged); err != nil {
			return err
		}
		if charged && (before.DueDay != req.DueDay || !equalMonths(before.BillingMonths, req.BillingMonths)) {
			return httpx.Conflict("year_has_charges", "Já existem cobranças neste ano: os meses e o dia de vencimento não podem mudar.")
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE school_year SET starts_on=$2, ends_on=$3, billing_months=$4, due_day=$5, status=$6 WHERE id=$1`,
			id, req.StartsOn, req.EndsOn, req.BillingMonths, req.DueDay, req.Status); err != nil {
			return err
		}
		if out, err = h.loadYear(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "school_year", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func equalMonths(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// yearID resolves ?year=2027 (default: the open year, else the latest).
func yearID(ctx context.Context, q db.Querier, r *http.Request) (uuid.UUID, error) {
	var id uuid.UUID
	var err error
	if y := httpx.QueryInt(r, "year", 0); y > 0 {
		err = q.QueryRow(ctx, `SELECT id FROM school_year WHERE year = $1`, y).Scan(&id)
	} else {
		err = q.QueryRow(ctx, `SELECT id FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1`).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, httpx.NotFound()
	}
	return id, err
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
