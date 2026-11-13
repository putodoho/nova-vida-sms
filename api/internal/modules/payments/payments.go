// Package payments records payments, allocates them to charges, issues receipt
// numbers and cancels payments. Payments are never deleted.
package payments

import (
	"context"
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
	"github.com/putodoho/nova-vida-sms/api/internal/numbering"
)

type Handler struct {
	Pool *pgxpool.Pool
	Loc  *time.Location
	Now  func() time.Time
}

// Routes are entity-scoped.
func (h *Handler) Routes(r chi.Router) {
	create := auth.Require(auth.PaymentsCreate)
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/payments", httpx.Handler(h.list))
	r.With(create).Method(http.MethodPost, "/payments/preview", httpx.Handler(h.preview))
	r.With(create).Method(http.MethodPost, "/payments", httpx.Handler(h.create))
	r.With(create).Method(http.MethodPost, "/payments/group", httpx.Handler(h.createGroup))
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/payments/{id}", httpx.Handler(h.get))
	r.With(auth.Require(auth.PaymentsCancel)).Method(http.MethodPost, "/payments/{id}/cancel", httpx.Handler(h.cancel))
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

// Maximum single payment accepted (sanity check against typing errors): 1.000.000 MT.
const maxAmount = money.Amount(100_000_000)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type AllocationLine struct {
	ChargeID    uuid.UUID    `json:"charge_id"`
	Description string       `json:"description"`
	DueDate     string       `json:"due_date"`
	Kind        string       `json:"kind"`
	Amount      money.Amount `json:"amount"`
	Active      bool         `json:"active"`
}

type Payment struct {
	ID                 uuid.UUID        `json:"id"`
	EntityID           uuid.UUID        `json:"entity_id"`
	PupilID            uuid.UUID        `json:"pupil_id"`
	PupilName          string           `json:"pupil_name"`
	PupilNumber        string           `json:"pupil_number"`
	PaymentGroupID     *uuid.UUID       `json:"payment_group_id"`
	ReceiptNo          string           `json:"receipt_no"`
	PaidOn             string           `json:"paid_on"`
	Amount             money.Amount     `json:"amount"`
	Method             string           `json:"method"`
	ReceivingAccountID *uuid.UUID       `json:"receiving_account_id"`
	AccountLabel       *string          `json:"account_label"`
	Reference          string           `json:"reference"`
	Notes              string           `json:"notes"`
	ReceivedBy         uuid.UUID        `json:"received_by"`
	ReceivedByName     string           `json:"received_by_name"`
	CreatedAt          time.Time        `json:"created_at"`
	CancelledAt        *time.Time       `json:"cancelled_at"`
	CancelReason       string           `json:"cancel_reason"`
	Allocated          money.Amount     `json:"allocated"`
	Credit             money.Amount     `json:"credit"` // unallocated part of this payment
	Allocations        []AllocationLine `json:"allocations,omitempty"`
	// Pupil balance now (for the receipt and the success screen).
	PupilOverdue     *money.Amount `json:"pupil_overdue,omitempty"`
	PupilOutstanding *money.Amount `json:"pupil_outstanding,omitempty"`
}

const paymentSelect = `
	SELECT p.id, p.entity_id, p.pupil_id, pu.full_name, pu.number, p.payment_group_id, p.receipt_no, p.paid_on,
	       p.amount, p.method, p.receiving_account_id, ra.label, p.reference, p.notes, p.received_by, u.display_name,
	       p.created_at, p.cancelled_at, p.cancel_reason,
	       COALESCE((SELECT sum(a.amount) FROM allocation a WHERE a.payment_id = p.id AND a.active), 0)::bigint
	FROM payment p
	JOIN pupil pu ON pu.id = p.pupil_id
	JOIN app_user u ON u.id = p.received_by
	LEFT JOIN receiving_account ra ON ra.id = p.receiving_account_id`

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	var paidOn time.Time
	err := row.Scan(&p.ID, &p.EntityID, &p.PupilID, &p.PupilName, &p.PupilNumber, &p.PaymentGroupID, &p.ReceiptNo, &paidOn,
		&p.Amount, &p.Method, &p.ReceivingAccountID, &p.AccountLabel, &p.Reference, &p.Notes, &p.ReceivedBy, &p.ReceivedByName,
		&p.CreatedAt, &p.CancelledAt, &p.CancelReason, &p.Allocated)
	p.PaidOn = paidOn.Format(time.DateOnly)
	if p.CancelledAt == nil {
		p.Credit = p.Amount - p.Allocated
	}
	return p, err
}

func (h *Handler) load(ctx context.Context, q db.Querier, entityIDs []uuid.UUID, id uuid.UUID) (Payment, error) {
	p, err := scanPayment(q.QueryRow(ctx, paymentSelect+` WHERE p.id = $1 AND p.entity_id = ANY($2)`, id, entityIDs))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.NotFound()
	}
	if err != nil {
		return p, err
	}
	rows, err := q.Query(ctx, `
		SELECT a.charge_id, c.description, c.due_date, c.kind, a.amount, a.active
		FROM allocation a JOIN charge c ON c.id = a.charge_id
		WHERE a.payment_id = $1 ORDER BY c.due_date, c.kind`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	p.Allocations = []AllocationLine{}
	for rows.Next() {
		var a AllocationLine
		var due time.Time
		if err := rows.Scan(&a.ChargeID, &a.Description, &due, &a.Kind, &a.Amount, &a.Active); err != nil {
			return p, err
		}
		a.DueDate = due.Format(time.DateOnly)
		p.Allocations = append(p.Allocations, a)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	var overdue, outstanding money.Amount
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(remaining) FILTER (WHERE due_date < $3), 0)::bigint, COALESCE(sum(remaining), 0)::bigint
		FROM v_charge_balance WHERE entity_id = $1 AND pupil_id = $2 AND NOT cancelled`,
		p.EntityID, p.PupilID, h.today()).Scan(&overdue, &outstanding); err != nil {
		return p, err
	}
	p.PupilOverdue, p.PupilOutstanding = &overdue, &outstanding
	return p, nil
}

// ---------------------------------------------------------------------------
// Validation shared by preview, create and group
// ---------------------------------------------------------------------------

type paymentInput struct {
	PupilID            uuid.UUID    `json:"pupil_id"`
	Amount             money.Amount `json:"amount"`
	PaidOn             string       `json:"paid_on"` // default today
	Method             string       `json:"method"`
	ReceivingAccountID *uuid.UUID   `json:"receiving_account_id"`
	Reference          string       `json:"reference"`
	Notes              string       `json:"notes"`
}

func (h *Handler) validateCommon(ctx context.Context, q db.Querier, entityID uuid.UUID, in *paymentInput, f httpx.Fields) (time.Time, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.Notes = strings.TrimSpace(in.Notes)
	today := h.today()
	paidOn := today
	if in.PaidOn != "" {
		d, err := time.Parse(time.DateOnly, in.PaidOn)
		f.Add(err != nil, "paid_on", "Data inválida.")
		if err == nil {
			paidOn = d
		}
	}
	f.Add(paidOn.After(today), "paid_on", "A data do pagamento não pode ser futura.")
	f.Add(paidOn.Before(today.AddDate(-1, 0, 0)), "paid_on", "Data com mais de um ano.")
	switch in.Method {
	case "cash", "bank", "mpesa", "emola":
	default:
		f.Add(true, "method", "Método inválido.")
	}
	f.Add(in.Method != "cash" && in.Reference == "", "reference", "Indique a referência da transacção.")
	if in.ReceivingAccountID == nil {
		f.Add(true, "receiving_account_id", "Indique a conta que recebeu o dinheiro.")
	} else {
		var kind string
		var active bool
		err := q.QueryRow(ctx, `SELECT kind, active FROM receiving_account WHERE id = $1 AND entity_id = $2`, in.ReceivingAccountID, entityID).Scan(&kind, &active)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			f.Add(true, "receiving_account_id", "Conta inválida para esta empresa.")
		case err != nil:
			return paidOn, err
		default:
			f.Add(!active, "receiving_account_id", "Conta desactivada.")
			f.Add(kind != in.Method, "receiving_account_id", "A conta não corresponde ao método de pagamento.")
		}
	}
	return paidOn, nil
}

func validatePupil(ctx context.Context, q db.Querier, entityID, pupilID uuid.UUID, amount money.Amount, f httpx.Fields, prefix string) error {
	f.Add(amount <= 0, prefix+"amount", "Valor inválido.")
	f.Add(amount > maxAmount, prefix+"amount", "Valor demasiado alto; confirme.")
	var enrolled bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrolment WHERE pupil_id = $1 AND entity_id = $2 AND status <> 'draft')`,
		pupilID, entityID).Scan(&enrolled); err != nil {
		return err
	}
	f.Add(!enrolled, prefix+"pupil_id", "O aluno não tem matrícula nesta empresa.")
	return nil
}

// checkReference refuses a second payment with the same method and reference
// (e.g. the same M-Pesa transaction registered twice).
func checkReference(ctx context.Context, q db.Querier, entityID uuid.UUID, method, ref string) error {
	if method == "cash" || ref == "" {
		return nil
	}
	var receipt string
	err := q.QueryRow(ctx, `
		SELECT receipt_no FROM payment
		WHERE entity_id = $1 AND method = $2 AND lower(reference) = lower($3) AND cancelled_at IS NULL LIMIT 1`,
		entityID, method, ref).Scan(&receipt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return httpx.Conflict("duplicate_reference", "Esta referência já foi registada no recibo "+receipt+".")
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

type previewLine struct {
	ChargeID    uuid.UUID    `json:"charge_id"`
	Description string       `json:"description"`
	DueDate     string       `json:"due_date"`
	Kind        string       `json:"kind"`
	Remaining   money.Amount `json:"remaining"` // before this payment
	Amount      money.Amount `json:"amount"`    // taken by this payment
}

type previewResponse struct {
	Open        []previewLine `json:"open"` // all open charges, oldest first
	Allocations []previewLine `json:"allocations"`
	Credit      money.Amount  `json:"credit"`
	Overdue     money.Amount  `json:"overdue"`
	Outstanding money.Amount  `json:"outstanding"`
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		PupilID uuid.UUID    `json:"pupil_id"`
		Amount  money.Amount `json:"amount"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var resp previewResponse
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		f := httpx.Fields{}
		if err := validatePupil(r.Context(), tx, entityID, in.PupilID, max(in.Amount, 1), f, ""); err != nil {
			return err
		}
		f.Add(in.Amount < 0, "amount", "Valor inválido.")
		if err := f.Err(); err != nil {
			return err
		}
		open, desc, err := ledger.OpenCharges(r.Context(), tx, entityID, in.PupilID)
		if err != nil {
			return err
		}
		today := h.today()
		resp.Open, resp.Allocations = []previewLine{}, []previewLine{}
		byID := map[uuid.UUID]billing.OpenCharge{}
		for _, c := range open {
			byID[c.ID] = c
			resp.Open = append(resp.Open, previewLine{c.ID, desc[c.ID], c.DueDate.Format(time.DateOnly), c.Kind, c.Remaining, 0})
			resp.Outstanding += c.Remaining
			if c.DueDate.Before(today) {
				resp.Overdue += c.Remaining
			}
		}
		allocs, credit := billing.Allocate(in.Amount, append([]billing.OpenCharge(nil), open...))
		for _, a := range allocs {
			c := byID[a.ChargeID]
			resp.Allocations = append(resp.Allocations, previewLine{c.ID, desc[c.ID], c.DueDate.Format(time.DateOnly), c.Kind, c.Remaining, a.Amount})
		}
		resp.Credit = credit
		return nil
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, resp)
	return nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var in paymentInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var out Payment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		f := httpx.Fields{}
		paidOn, err := h.validateCommon(r.Context(), tx, entityID, &in, f)
		if err != nil {
			return err
		}
		if err := validatePupil(r.Context(), tx, entityID, in.PupilID, in.Amount, f, ""); err != nil {
			return err
		}
		if err := f.Err(); err != nil {
			return err
		}
		if err := checkReference(r.Context(), tx, entityID, in.Method, in.Reference); err != nil {
			return err
		}
		id, err := h.insert(r.Context(), tx, entityID, in, paidOn, nil)
		if err != nil {
			return err
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

// insert writes one payment, its receipt number and allocations, and audits it.
func (h *Handler) insert(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, in paymentInput, paidOn time.Time, groupID *uuid.UUID) (uuid.UUID, error) {
	if err := ledger.LockPupil(ctx, tx, entityID, in.PupilID); err != nil {
		return uuid.Nil, err
	}
	receipt, err := numbering.Document(ctx, tx, entityID, "receipt", paidOn.Year())
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO payment (entity_id, pupil_id, payment_group_id, receipt_no, paid_on, amount, method,
		                     receiving_account_id, reference, notes, received_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		entityID, in.PupilID, groupID, receipt, paidOn, in.Amount, in.Method, in.ReceivingAccountID, in.Reference, in.Notes,
		auth.CurrentUser(ctx).ID).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	allocs, credit, err := ledger.Allocate(ctx, tx, entityID, in.PupilID, id, in.Amount)
	if err != nil {
		return uuid.Nil, err
	}
	return id, audit.Write(ctx, tx, audit.Entry{EntityID: &entityID, Action: "create", Table: "payment", RecordID: id.String(),
		After: map[string]any{"receipt_no": receipt, "pupil_id": in.PupilID, "amount": in.Amount, "method": in.Method,
			"reference": in.Reference, "paid_on": paidOn.Format(time.DateOnly), "allocations": allocs, "credit": credit}})
}

type groupRequest struct {
	PaidOn             string     `json:"paid_on"`
	Method             string     `json:"method"`
	ReceivingAccountID *uuid.UUID `json:"receiving_account_id"`
	Reference          string     `json:"reference"`
	Notes              string     `json:"notes"`
	Splits             []struct {
		PupilID uuid.UUID    `json:"pupil_id"`
		Amount  money.Amount `json:"amount"`
	} `json:"splits"`
}

// createGroup records one family payment split between children (same
// entity): one payment and receipt per child, sharing a payment_group_id.
func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req groupRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out []Payment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		base := paymentInput{PaidOn: req.PaidOn, Method: req.Method, ReceivingAccountID: req.ReceivingAccountID, Reference: req.Reference, Notes: req.Notes}
		f := httpx.Fields{}
		paidOn, err := h.validateCommon(r.Context(), tx, entityID, &base, f)
		if err != nil {
			return err
		}
		f.Add(len(req.Splits) < 2 || len(req.Splits) > 8, "splits", "Indique entre 2 e 8 alunos.")
		seen := map[uuid.UUID]bool{}
		for _, s := range req.Splits {
			f.Add(seen[s.PupilID], "splits.pupil_id", "Aluno repetido.")
			seen[s.PupilID] = true
			if err := validatePupil(r.Context(), tx, entityID, s.PupilID, s.Amount, f, "splits."); err != nil {
				return err
			}
		}
		if err := f.Err(); err != nil {
			return err
		}
		if err := checkReference(r.Context(), tx, entityID, base.Method, base.Reference); err != nil {
			return err
		}
		group := uuid.New()
		for _, s := range req.Splits {
			in := base
			in.PupilID, in.Amount = s.PupilID, s.Amount
			id, err := h.insert(r.Context(), tx, entityID, in, paidOn, &group)
			if err != nil {
				return err
			}
			p, err := h.load(r.Context(), tx, []uuid.UUID{entityID}, id)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, httpx.List[Payment]{Items: out, Total: len(out)})
	return nil
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := h.load(r.Context(), h.Pool, auth.EntityScope(r.Context()).EntityIDs, id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

type listResponse struct {
	Items    []Payment               `json:"items"`
	Total    int                     `json:"total"`
	Sum      money.Amount            `json:"sum"`       // valid payments matching the filters
	ByMethod map[string]money.Amount `json:"by_method"` // valid payments by method
}

// list: filters from, to (paid_on), method, receiving_account_id, received_by,
// pupil_id, status (valid|cancelled), q (receipt number or pupil name).
func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	qs := r.URL.Query()
	page := httpx.ParsePage(r)
	parseDate := func(name string) (*time.Time, error) {
		v := qs.Get(name)
		if v == "" {
			return nil, nil
		}
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return nil, httpx.BadRequest(name + " inválido (AAAA-MM-DD).")
		}
		return &d, nil
	}
	parseID := func(name string) (*uuid.UUID, error) {
		v := qs.Get(name)
		if v == "" {
			return nil, nil
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, httpx.BadRequest(name + " inválido.")
		}
		return &id, nil
	}
	from, err := parseDate("from")
	if err != nil {
		return err
	}
	to, err := parseDate("to")
	if err != nil {
		return err
	}
	account, err := parseID("receiving_account_id")
	if err != nil {
		return err
	}
	receivedBy, err := parseID("received_by")
	if err != nil {
		return err
	}
	pupil, err := parseID("pupil_id")
	if err != nil {
		return err
	}
	where := `
		WHERE p.entity_id = ANY($1)
		  AND ($2::date IS NULL OR p.paid_on >= $2) AND ($3::date IS NULL OR p.paid_on <= $3)
		  AND ($4 = '' OR p.method = $4)
		  AND ($5::uuid IS NULL OR p.receiving_account_id = $5)
		  AND ($6::uuid IS NULL OR p.received_by = $6)
		  AND ($7::uuid IS NULL OR p.pupil_id = $7)
		  AND ($8 = '' OR ($8 = 'valid' AND p.cancelled_at IS NULL) OR ($8 = 'cancelled' AND p.cancelled_at IS NOT NULL))
		  AND ($9 = '' OR p.receipt_no ILIKE '%' || $9 || '%' OR pu.full_name ILIKE '%' || $9 || '%' OR p.reference ILIKE '%' || $9 || '%')`
	args := []any{auth.EntityScope(r.Context()).EntityIDs, from, to, qs.Get("method"), account, receivedBy, pupil,
		qs.Get("status"), strings.TrimSpace(qs.Get("q"))}

	resp := listResponse{Items: []Payment{}, ByMethod: map[string]money.Amount{}}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.method, count(*) OVER ()::int, sum(p.amount) FILTER (WHERE p.cancelled_at IS NULL) OVER (PARTITION BY p.method)
		FROM payment p JOIN pupil pu ON pu.id = p.pupil_id`+where, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var method string
		var sum *money.Amount
		if err := rows.Scan(&method, &resp.Total, &sum); err != nil {
			rows.Close()
			return err
		}
		if sum != nil {
			resp.ByMethod[method] = *sum
		}
	}
	rows.Close()
	for _, v := range resp.ByMethod {
		resp.Sum += v
	}

	rows, err = h.Pool.Query(r.Context(), paymentSelect+where+`
		ORDER BY p.paid_on DESC, p.created_at DESC LIMIT $10 OFFSET $11`, append(args, page.PageSize, page.Offset())...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return err
		}
		resp.Items = append(resp.Items, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, resp)
	return nil
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// cancel voids a payment: the receipt keeps its number and shows as ANULADO,
// allocations become inactive, and other payments' credit is re-applied.
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 5 {
		return httpx.Validation(httpx.Fields{"reason": "Indique o motivo da anulação."})
	}
	var out Payment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := h.load(r.Context(), tx, []uuid.UUID{entityID}, id)
		if err != nil {
			return err
		}
		if before.CancelledAt != nil {
			return httpx.Conflict("already_cancelled", "Este pagamento já foi anulado.")
		}
		if err := ledger.LockPupil(r.Context(), tx, entityID, before.PupilID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE payment SET cancelled_at = now(), cancelled_by = $2, cancel_reason = $3 WHERE id = $1`,
			id, auth.CurrentUser(r.Context()).ID, req.Reason); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE allocation SET active = false WHERE payment_id = $1`, id); err != nil {
			return err
		}
		if err := ledger.ApplyCredit(r.Context(), tx, entityID, before.PupilID); err != nil {
			return err
		}
		if out, err = h.load(r.Context(), tx, []uuid.UUID{entityID}, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "cancel", Table: "payment", RecordID: id.String(),
			Before: map[string]any{"receipt_no": before.ReceiptNo, "amount": before.Amount, "allocations": before.Allocations},
			After:  map[string]any{"reason": req.Reason}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}
