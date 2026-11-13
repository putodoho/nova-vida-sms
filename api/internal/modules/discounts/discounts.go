// Package discounts handles discount requests and their approval. An approved
// request becomes one discount row per affected unpaid charge.
package discounts

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/putodoho/nova-vida-sms/api/internal/ledger"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/discount-requests", httpx.Handler(h.list))
	r.With(auth.Require(auth.DiscountsRequest)).Method(http.MethodPost, "/discount-requests", httpx.Handler(h.create))
	r.With(auth.Require(auth.DiscountsApprove)).Method(http.MethodPost, "/discount-requests/{id}/approve", httpx.Handler(h.approve))
	r.With(auth.Require(auth.DiscountsApprove)).Method(http.MethodPost, "/discount-requests/{id}/reject", httpx.Handler(h.reject))
}

// Scope selects the charges a discount applies to.
type Scope struct {
	FromMonth int `json:"from_month"` // first billing month; earlier months are untouched
}

type Request struct {
	ID              uuid.UUID    `json:"id"`
	EntityID        uuid.UUID    `json:"entity_id"`
	EnrolmentID     uuid.UUID    `json:"enrolment_id"`
	PupilID         uuid.UUID    `json:"pupil_id"`
	PupilName       string       `json:"pupil_name"`
	ClassName       *string      `json:"class_name"`
	DiscountTypeID  uuid.UUID    `json:"discount_type_id"`
	DiscountType    string       `json:"discount_type"`
	FeeItemCodes    []string     `json:"fee_item_codes"`
	Mode            string       `json:"mode"`
	Value           int64        `json:"value"`
	Scope           Scope        `json:"scope"`
	Reason          string       `json:"reason"`
	Status          string       `json:"status"`
	RequestedBy     string       `json:"requested_by"`
	DecidedBy       *string      `json:"decided_by"`
	DecidedAt       *time.Time   `json:"decided_at"`
	DecisionNote    string       `json:"decision_note"`
	CreatedAt       time.Time    `json:"created_at"`
	EstimatedAmount money.Amount `json:"estimated_amount"` // pending: what approval would grant now; approved: granted
}

const requestSelect = `
	SELECT dr.id, dr.entity_id, dr.enrolment_id, e.pupil_id, p.full_name, cl.name, dr.discount_type_id, dt.name, dt.fee_item_codes,
	       dr.mode, dr.value, dr.scope, dr.reason, dr.status, ru.display_name, du.display_name, dr.decided_at, dr.decision_note, dr.created_at
	FROM discount_request dr
	JOIN enrolment e ON e.id = dr.enrolment_id
	JOIN pupil p ON p.id = e.pupil_id
	LEFT JOIN class cl ON cl.id = e.class_id
	JOIN discount_type dt ON dt.id = dr.discount_type_id
	JOIN app_user ru ON ru.id = dr.requested_by
	LEFT JOIN app_user du ON du.id = dr.decided_by`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	var scope []byte
	err := row.Scan(&r.ID, &r.EntityID, &r.EnrolmentID, &r.PupilID, &r.PupilName, &r.ClassName, &r.DiscountTypeID, &r.DiscountType,
		&r.FeeItemCodes, &r.Mode, &r.Value, &scope, &r.Reason, &r.Status, &r.RequestedBy, &r.DecidedBy, &r.DecidedAt, &r.DecisionNote, &r.CreatedAt)
	_ = json.Unmarshal(scope, &r.Scope)
	return r, err
}

// targets returns the unpaid charges a request applies to, with their discount.
type target struct {
	chargeID uuid.UUID
	amount   money.Amount
}

func targets(ctx context.Context, q db.Querier, req Request) ([]target, money.Amount, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, c.amount_gross, b.remaining
		FROM charge c
		JOIN v_charge_balance b ON b.charge_id = c.id
		JOIN fee_item f ON f.id = c.fee_item_id
		WHERE c.enrolment_id = $1 AND c.kind = 'monthly' AND c.cancelled_at IS NULL AND b.remaining > 0
		  AND f.code = ANY($2) AND c.period_month >= $3
		ORDER BY c.due_date`, req.EnrolmentID, req.FeeItemCodes, req.Scope.FromMonth)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []target
	var total money.Amount
	for rows.Next() {
		var id uuid.UUID
		var gross, remaining money.Amount
		if err := rows.Scan(&id, &gross, &remaining); err != nil {
			return nil, 0, err
		}
		if amt := billing.DiscountAmount(req.Mode, req.Value, gross, remaining); amt > 0 {
			out = append(out, target{id, amt})
			total += amt
		}
	}
	return out, total, rows.Err()
}

func (h *Handler) load(ctx context.Context, q db.Querier, entityIDs []uuid.UUID, id uuid.UUID) (Request, error) {
	r, err := scanRequest(q.QueryRow(ctx, requestSelect+` WHERE dr.id = $1 AND dr.entity_id = ANY($2)`, id, entityIDs))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, httpx.NotFound()
	}
	if err != nil {
		return r, err
	}
	return r, h.fillAmount(ctx, q, &r)
}

func (h *Handler) fillAmount(ctx context.Context, q db.Querier, r *Request) error {
	switch r.Status {
	case "pending":
		_, total, err := targets(ctx, q, *r)
		r.EstimatedAmount = total
		return err
	case "approved":
		return q.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0)::bigint FROM discount WHERE discount_request_id = $1`, r.ID).Scan(&r.EstimatedAmount)
	}
	return nil
}

// list: filters status, enrolment_id.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	status := r.URL.Query().Get("status")
	var enrolment *uuid.UUID
	if v := r.URL.Query().Get("enrolment_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return httpx.BadRequest("enrolment_id inválido.")
		}
		enrolment = &id
	}
	rows, err := h.Pool.Query(r.Context(), requestSelect+`
		WHERE dr.entity_id = ANY($1) AND ($2 = '' OR dr.status = $2) AND ($3::uuid IS NULL OR dr.enrolment_id = $3)
		ORDER BY (dr.status = 'pending') DESC, dr.created_at DESC LIMIT 200`,
		auth.EntityScope(r.Context()).EntityIDs, status, enrolment)
	if err != nil {
		return err
	}
	items := []Request{}
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, req)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if err := h.fillAmount(r.Context(), h.Pool, &items[i]); err != nil {
			return err
		}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Request]{Items: items, Total: len(items)})
	return nil
}

type createRequest struct {
	EnrolmentID    uuid.UUID `json:"enrolment_id"`
	DiscountTypeID uuid.UUID `json:"discount_type_id"`
	Mode           string    `json:"mode"`  // default: the type's
	Value          *int64    `json:"value"` // default: the type's
	FromMonth      int       `json:"from_month"`
	Reason         string    `json:"reason"`
}

// create files a request. Types that do not require approval are approved
// immediately.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var in createRequest
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var out Request
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var typeMode string
		var typeValue int64
		var needsApproval, active bool
		err := tx.QueryRow(r.Context(), `SELECT mode, value, requires_approval, active FROM discount_type WHERE id = $1 AND entity_id = $2`,
			in.DiscountTypeID, entityID).Scan(&typeMode, &typeValue, &needsApproval, &active)
		f := httpx.Fields{}
		f.Add(errors.Is(err, pgx.ErrNoRows) || (err == nil && !active), "discount_type_id", "Tipo de desconto inválido.")
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var status string
		var startMonth int
		err = tx.QueryRow(r.Context(), `SELECT status, start_month FROM enrolment WHERE id = $1 AND entity_id = $2`, in.EnrolmentID, entityID).Scan(&status, &startMonth)
		f.Add(err != nil || status != "confirmed", "enrolment_id", "Matrícula confirmada inválida.")
		if in.Mode == "" {
			in.Mode = typeMode
		}
		value := typeValue
		if in.Value != nil {
			value = *in.Value
		}
		f.Add(in.Mode != "percent" && in.Mode != "fixed", "mode", "Modo inválido.")
		f.Add(value <= 0 || (in.Mode == "percent" && value > 10_000), "value", "Indique o valor do desconto.")
		if in.FromMonth == 0 {
			in.FromMonth = startMonth
		}
		f.Add(in.FromMonth < 1 || in.FromMonth > 12, "from_month", "Mês inválido.")
		in.Reason = strings.TrimSpace(in.Reason)
		f.Add(len(in.Reason) < 5, "reason", "Indique o motivo.")
		if err := f.Err(); err != nil {
			return err
		}
		scope, _ := json.Marshal(Scope{FromMonth: in.FromMonth})
		var id uuid.UUID
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO discount_request (entity_id, enrolment_id, discount_type_id, mode, value, scope, reason, requested_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			entityID, in.EnrolmentID, in.DiscountTypeID, in.Mode, value, scope, in.Reason, auth.CurrentUser(r.Context()).ID).Scan(&id); err != nil {
			return err
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "create", Table: "discount_request", RecordID: id.String(), After: in}); err != nil {
			return err
		}
		if !needsApproval {
			if err := h.decide(r.Context(), tx, entityID, id, true, "Aprovação automática"); err != nil {
				return err
			}
		}
		out, err = h.load(r.Context(), tx, []uuid.UUID{entityID}, id)
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

// decide approves or rejects a pending request. Approval writes the discounts
// on the unpaid charges as they are now.
func (h *Handler) decide(ctx context.Context, tx pgx.Tx, entityID, id uuid.UUID, approve bool, note string) error {
	req, err := scanRequest(tx.QueryRow(ctx, requestSelect+` WHERE dr.id = $1 AND dr.entity_id = $2 FOR UPDATE OF dr`, id, entityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound()
	}
	if err != nil {
		return err
	}
	if req.Status != "pending" {
		return httpx.Conflict("already_decided", "Este pedido já foi decidido.")
	}
	status := "rejected"
	var granted money.Amount
	if approve {
		status = "approved"
		if err := ledger.LockPupil(ctx, tx, entityID, req.PupilID); err != nil {
			return err
		}
		ts, total, err := targets(ctx, tx, req)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if _, err := tx.Exec(ctx, `INSERT INTO discount (charge_id, discount_request_id, amount) VALUES ($1, $2, $3)`, t.chargeID, id, t.amount); err != nil {
				return err
			}
		}
		granted = total
	}
	if _, err := tx.Exec(ctx, `
		UPDATE discount_request SET status = $2, decided_by = $3, decided_at = now(), decision_note = $4 WHERE id = $1`,
		id, status, auth.CurrentUser(ctx).ID, note); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{EntityID: &entityID, Action: status, Table: "discount_request", RecordID: id.String(),
		After: map[string]any{"note": note, "granted": granted}})
}

func (h *Handler) decideHandler(w http.ResponseWriter, r *http.Request, approve bool) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Note = strings.TrimSpace(in.Note)
	if !approve && len(in.Note) < 5 {
		return httpx.Validation(httpx.Fields{"note": "Indique o motivo da recusa."})
	}
	user := auth.CurrentUser(r.Context())
	var out Request
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var requestedBy uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT requested_by FROM discount_request WHERE id = $1 AND entity_id = $2`, id, entityID).Scan(&requestedBy); err != nil {
			return httpx.NotFound()
		}
		// Separation of duties: only an administrator may decide their own request.
		if requestedBy == user.ID && !user.Can(auth.UsersManage) {
			return httpx.Conflict("own_request", "Não pode decidir um pedido feito por si.")
		}
		if err := h.decide(r.Context(), tx, entityID, id, approve, in.Note); err != nil {
			return err
		}
		var err error
		out, err = h.load(r.Context(), tx, []uuid.UUID{entityID}, id)
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) error {
	return h.decideHandler(w, r, true)
}
func (h *Handler) reject(w http.ResponseWriter, r *http.Request) error {
	return h.decideHandler(w, r, false)
}
