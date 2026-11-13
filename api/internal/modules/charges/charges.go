// Package charges lists charges and arrears, cancels charges, waives
// penalties and runs the penalty job on demand.
package charges

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/jobs"
	"github.com/putodoho/nova-vida-sms/api/internal/ledger"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

type Handler struct {
	Pool *pgxpool.Pool
	Loc  *time.Location
	Now  func() time.Time
}

func (h *Handler) Routes(r chi.Router) {
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/charges", httpx.Handler(h.list))
	r.With(auth.Require(auth.ChargesCancel)).Method(http.MethodPost, "/charges/{id}/cancel", httpx.Handler(h.cancel))
	r.With(auth.Require(auth.PenaltiesWaive)).Method(http.MethodPost, "/penalties/{id}/waive", httpx.Handler(h.waive))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPost, "/penalties/run", httpx.Handler(h.runPenalties))
}

func (h *Handler) today() time.Time {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	loc := h.Loc
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type Charge struct {
	ID          uuid.UUID    `json:"id"`
	EntityID    uuid.UUID    `json:"entity_id"`
	PupilID     uuid.UUID    `json:"pupil_id"`
	PupilName   string       `json:"pupil_name"`
	PupilNumber string       `json:"pupil_number"`
	ClassName   *string      `json:"class_name"`
	PayerPhone  *string      `json:"payer_phone"`
	Kind        string       `json:"kind"`
	PeriodMonth *int         `json:"period_month"`
	Description string       `json:"description"`
	DueDate     string       `json:"due_date"`
	Gross       money.Amount `json:"gross"`
	Discount    money.Amount `json:"discount"`
	Paid        money.Amount `json:"paid"`
	Remaining   money.Amount `json:"remaining"`
	DaysOverdue int          `json:"days_overdue"`
	Status      string       `json:"status"`
}

type Aging struct {
	D1to30  money.Amount `json:"d1_30"`
	D31to60 money.Amount `json:"d31_60"`
	D61to90 money.Amount `json:"d61_90"`
	Over90  money.Amount `json:"over_90"`
	Total   money.Amount `json:"total"`
	Pupils  int          `json:"pupils"`
}

type listResponse struct {
	Items []Charge `json:"items"`
	Total int      `json:"total"`
	Aging Aging    `json:"aging"` // overdue amounts in scope (all filters except paging)
}

// list: filters year, month (period month), class_id, kind, status
// (open|overdue|paid|cancelled), min_days (overdue at least N days), q.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	qs := r.URL.Query()
	page := httpx.ParsePage(r)
	today := h.today()
	var classID *uuid.UUID
	if v := qs.Get("class_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return httpx.BadRequest("class_id inválido.")
		}
		classID = &id
	}
	status := qs.Get("status")
	switch status {
	case "", "open", "overdue", "paid", "cancelled":
	default:
		return httpx.BadRequest("status inválido.")
	}
	minDays := httpx.QueryInt(r, "min_days", 0)
	where := `
		FROM charge c
		JOIN v_charge_balance b ON b.charge_id = c.id
		JOIN enrolment e ON e.id = c.enrolment_id
		JOIN school_year sy ON sy.id = e.school_year_id
		JOIN pupil p ON p.id = c.pupil_id
		LEFT JOIN class cl ON cl.id = e.class_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE c.entity_id = ANY($1)
		  AND (($2 = 0 AND sy.id = (SELECT id FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1)) OR sy.year = $2)
		  AND ($3 = 0 OR c.period_month = $3)
		  AND ($4::uuid IS NULL OR e.class_id = $4)
		  AND ($5 = '' OR c.kind = $5)
		  AND (CASE $6
		         WHEN 'open' THEN NOT b.cancelled AND b.remaining > 0
		         WHEN 'overdue' THEN NOT b.cancelled AND b.remaining > 0 AND c.due_date < $7
		         WHEN 'paid' THEN NOT b.cancelled AND b.remaining = 0
		         WHEN 'cancelled' THEN b.cancelled
		         ELSE true END)
		  AND ($8 = 0 OR (NOT b.cancelled AND b.remaining > 0 AND $7::date - c.due_date >= $8))
		  AND ($9 = '' OR p.full_name ILIKE '%' || $9 || '%' OR p.number ILIKE $9 || '%')`
	args := []any{auth.EntityScope(r.Context()).EntityIDs, httpx.QueryInt(r, "year", 0), httpx.QueryInt(r, "month", 0),
		classID, qs.Get("kind"), status, today, minDays, strings.TrimSpace(qs.Get("q"))}

	resp := listResponse{Items: []Charge{}}
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT count(*)::int,
		       COALESCE(sum(b.remaining) FILTER (WHERE od BETWEEN 1 AND 30), 0)::bigint,
		       COALESCE(sum(b.remaining) FILTER (WHERE od BETWEEN 31 AND 60), 0)::bigint,
		       COALESCE(sum(b.remaining) FILTER (WHERE od BETWEEN 61 AND 90), 0)::bigint,
		       COALESCE(sum(b.remaining) FILTER (WHERE od > 90), 0)::bigint,
		       count(DISTINCT c.pupil_id) FILTER (WHERE od > 0)::int
		FROM (SELECT c.*, CASE WHEN NOT b.cancelled AND b.remaining > 0 THEN $7::date - c.due_date ELSE 0 END AS od `+where+`) c
		JOIN v_charge_balance b ON b.charge_id = c.id`, args...).
		Scan(&resp.Total, &resp.Aging.D1to30, &resp.Aging.D31to60, &resp.Aging.D61to90, &resp.Aging.Over90, &resp.Aging.Pupils); err != nil {
		return err
	}
	resp.Aging.Total = resp.Aging.D1to30 + resp.Aging.D31to60 + resp.Aging.D61to90 + resp.Aging.Over90

	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id, c.entity_id, p.id, p.full_name, p.number, cl.name, g.phone1, c.kind, c.period_month, c.description,
		       c.due_date, c.amount_gross, b.discount, b.paid, b.remaining, b.cancelled, b.payable `+where+`
		ORDER BY c.due_date, p.full_name, c.kind
		LIMIT $10 OFFSET $11`, append(args, page.PageSize, page.Offset())...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c Charge
		var due time.Time
		var cancelled bool
		var payable money.Amount
		if err := rows.Scan(&c.ID, &c.EntityID, &c.PupilID, &c.PupilName, &c.PupilNumber, &c.ClassName, &c.PayerPhone, &c.Kind,
			&c.PeriodMonth, &c.Description, &due, &c.Gross, &c.Discount, &c.Paid, &c.Remaining, &cancelled, &payable); err != nil {
			return err
		}
		c.DueDate = due.Format(time.DateOnly)
		c.Status = billing.Status(cancelled, payable, c.Paid, due, today)
		if c.Status == billing.StatusOverdue {
			c.DaysOverdue = int(today.Sub(due).Hours() / 24)
		}
		resp.Items = append(resp.Items, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, resp)
	return nil
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

// cancelCharge cancels a charge in the caller's transaction. Money already
// allocated to it is released and re-applied to the pupil's other debts (or
// becomes credit). Penalties of a cancelled fee are cancelled with it.
func cancelCharge(r *http.Request, tx pgx.Tx, entityID, id uuid.UUID, wantKind func(string) bool, action, reason string) error {
	ctx := r.Context()
	var pupilID uuid.UUID
	var kind, desc string
	var cancelledAt *time.Time
	err := tx.QueryRow(ctx, `SELECT pupil_id, kind, description, cancelled_at FROM charge WHERE id = $1 AND entity_id = $2`, id, entityID).
		Scan(&pupilID, &kind, &desc, &cancelledAt)
	if err != nil {
		return httpx.NotFound()
	}
	if !wantKind(kind) {
		return httpx.Conflict("wrong_kind", "Esta operação não se aplica a este tipo de cobrança.")
	}
	if cancelledAt != nil {
		return httpx.Conflict("already_cancelled", "Esta cobrança já foi anulada.")
	}
	if err := ledger.LockPupil(ctx, tx, entityID, pupilID); err != nil {
		return err
	}
	userID := auth.CurrentUser(ctx).ID
	ids := []uuid.UUID{id}
	if kind != "penalty" {
		rows, err := tx.Query(ctx, `SELECT id FROM charge WHERE parent_charge_id = $1 AND cancelled_at IS NULL`, id)
		if err != nil {
			return err
		}
		children, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		ids = append(ids, children...)
	}
	var released money.Amount
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0)::bigint FROM allocation WHERE charge_id = ANY($1) AND active`, ids).Scan(&released); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocation SET active = false WHERE charge_id = ANY($1) AND active`, ids); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE charge SET cancelled_at = now(), cancelled_by = $2, cancel_reason = $3 WHERE id = ANY($1)`,
		ids, userID, reason); err != nil {
		return err
	}
	if err := ledger.ApplyCredit(ctx, tx, entityID, pupilID); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{EntityID: &entityID, Action: action, Table: "charge", RecordID: id.String(),
		After: map[string]any{"description": desc, "reason": reason, "cancelled": ids, "released": released}})
}

func (h *Handler) cancelOrWaive(w http.ResponseWriter, r *http.Request, wantKind func(string) bool, action string) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req reasonRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 5 {
		return httpx.Validation(httpx.Fields{"reason": "Indique o motivo."})
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		return cancelCharge(r, tx, entityID, id, wantKind, action, req.Reason)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) error {
	return h.cancelOrWaive(w, r, func(k string) bool { return k != "penalty" }, "cancel")
}

func (h *Handler) waive(w http.ResponseWriter, r *http.Request) error {
	return h.cancelOrWaive(w, r, func(k string) bool { return k == "penalty" }, "waive")
}

func (h *Handler) runPenalties(w http.ResponseWriter, r *http.Request) error {
	res, err := jobs.RunPenalties(r.Context(), h.Pool, h.today(), auth.EntityScope(r.Context()).EntityIDs)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}
