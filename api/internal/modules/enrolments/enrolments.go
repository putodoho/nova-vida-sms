// Package enrolments manages enrolments (draft → confirmed → withdrawn),
// contracted services, charge generation, renewals and the pupil account.
package enrolments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
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
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

type Handler struct {
	Pool *pgxpool.Pool
	Loc  *time.Location // school time zone, for "today"
	Now  func() time.Time
}

// Routes are entity-scoped.
func (h *Handler) Routes(r chi.Router) {
	write := auth.Require(auth.EnrolmentsWrite)
	read := auth.Require(auth.PupilsRead)

	r.With(read).Method(http.MethodGet, "/enrolments", httpx.Handler(h.list))
	r.With(write).Method(http.MethodPost, "/enrolments", httpx.Handler(h.create))
	r.With(write).Method(http.MethodPost, "/enrolments/renew", httpx.Handler(h.renew))
	r.With(read).Method(http.MethodGet, "/enrolments/{id}", httpx.Handler(h.get))
	r.With(write).Method(http.MethodPut, "/enrolments/{id}", httpx.Handler(h.update))
	r.With(write).Method(http.MethodDelete, "/enrolments/{id}", httpx.Handler(h.deleteDraft))
	r.With(write).Method(http.MethodPost, "/enrolments/{id}/confirm", httpx.Handler(h.confirm))
	r.With(write).Method(http.MethodPost, "/enrolments/{id}/withdraw", httpx.Handler(h.withdraw))
	r.With(write).Method(http.MethodPut, "/enrolments/{id}/services", httpx.Handler(h.changeServices))
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/pupils/{id}/account", httpx.Handler(h.account))
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

// Consents captured on the enrolment form.
type Consents struct {
	ImageUse      bool `json:"image_use"`
	FeesAgreement bool `json:"fees_agreement"`
	UniformBooks  bool `json:"uniform_books"`
	EmergencyCare bool `json:"emergency_care"`
}

type draftData struct {
	OneOffFeeIDs []uuid.UUID `json:"one_off_fee_ids"`
}

// Service is a contracted monthly service.
type Service struct {
	FeeItemID uuid.UUID    `json:"fee_item_id" db:"fee_item_id"`
	Code      string       `json:"code" db:"code"`
	Name      string       `json:"name" db:"name"`
	Amount    money.Amount `json:"amount" db:"amount"`
	Mandatory bool         `json:"mandatory" db:"mandatory"`
	FromMonth int          `json:"from_month" db:"from_month"`
	ToMonth   *int         `json:"to_month" db:"to_month"`
}

type Totals struct {
	MonthlyGross money.Amount `json:"monthly_gross"`
	MonthlyNet   money.Amount `json:"monthly_net"`
	MonthlyVAT   money.Amount `json:"monthly_vat"`
	AnnualGross  money.Amount `json:"annual_gross"` // monthly charges for the remaining months
	OneOffGross  money.Amount `json:"one_off_gross"`
	Months       int          `json:"months"`
}

type PreviewCharge struct {
	Description string      `json:"description"`
	Kind        string      `json:"kind"`
	PeriodMonth int         `json:"period_month"`
	DueDate     string      `json:"due_date"`
	Lines       money.Lines `json:"lines"`
}

type Enrolment struct {
	ID           uuid.UUID       `json:"id"`
	EntityID     uuid.UUID       `json:"entity_id"`
	SchoolYearID uuid.UUID       `json:"school_year_id"`
	Year         int             `json:"year"`
	PupilID      uuid.UUID       `json:"pupil_id"`
	PupilName    string          `json:"pupil_name"`
	PupilNumber  string          `json:"pupil_number"`
	ClassID      *uuid.UUID      `json:"class_id"`
	ClassName    *string         `json:"class_name"`
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	StartMonth   int             `json:"start_month"`
	Consents     Consents        `json:"consents"`
	ConfirmedAt  *time.Time      `json:"confirmed_at"`
	WithdrawnAt  *time.Time      `json:"withdrawn_at"`
	Services     []Service       `json:"services"`
	OneOffFeeIDs []uuid.UUID     `json:"one_off_fee_ids"`
	Totals       Totals          `json:"totals"`
	Preview      []PreviewCharge `json:"preview"` // drafts only: charges confirming would create
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

type ctxData struct {
	cal       billing.Calendar
	vatMethod string
}

func loadCalendar(ctx context.Context, q db.Querier, entityID, yearID uuid.UUID) (ctxData, error) {
	var c ctxData
	var months []int32
	err := q.QueryRow(ctx, `
		SELECT sy.year, sy.billing_months, sy.due_day, e.vat_method
		FROM school_year sy, entity e WHERE sy.id = $1 AND e.id = $2`, yearID, entityID).
		Scan(&c.cal.Year, &months, &c.cal.DueDay, &c.vatMethod)
	for _, m := range months {
		c.cal.BillingMonths = append(c.cal.BillingMonths, int(m))
	}
	return c, err
}

func (h *Handler) load(ctx context.Context, q db.Querier, entityID, id uuid.UUID, forUpdate bool) (*Enrolment, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF e"
	}
	e := &Enrolment{}
	var consents, draft []byte
	err := q.QueryRow(ctx, `
		SELECT e.id, e.entity_id, e.school_year_id, sy.year, e.pupil_id, p.full_name, p.number, e.class_id, c.name,
		       e.type, e.status, e.start_month, e.consents, e.draft_data, e.confirmed_at, e.withdrawn_at
		FROM enrolment e
		JOIN school_year sy ON sy.id = e.school_year_id
		JOIN pupil p ON p.id = e.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		WHERE e.id = $1 AND e.entity_id = $2`+lock, id, entityID).
		Scan(&e.ID, &e.EntityID, &e.SchoolYearID, &e.Year, &e.PupilID, &e.PupilName, &e.PupilNumber, &e.ClassID, &e.ClassName,
			&e.Type, &e.Status, &e.StartMonth, &consents, &draft, &e.ConfirmedAt, &e.WithdrawnAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(consents, &e.Consents)
	var dd draftData
	_ = json.Unmarshal(draft, &dd)
	e.OneOffFeeIDs = dd.OneOffFeeIDs
	if e.OneOffFeeIDs == nil {
		e.OneOffFeeIDs = []uuid.UUID{}
	}

	rows, err := q.Query(ctx, `
		SELECT cs.fee_item_id, f.code, f.name, f.amount, f.mandatory, cs.from_month, cs.to_month
		FROM contracted_service cs JOIN fee_item f ON f.id = cs.fee_item_id
		WHERE cs.enrolment_id = $1 ORDER BY f.mandatory DESC, f.code, cs.from_month`, id)
	if err != nil {
		return nil, err
	}
	if e.Services, err = pgx.CollectRows(rows, pgx.RowToStructByName[Service]); err != nil {
		return nil, err
	}
	if e.Services == nil {
		e.Services = []Service{}
	}
	if err := h.fillTotals(ctx, q, e); err != nil {
		return nil, err
	}
	return e, nil
}

// fillTotals computes the monthly/annual totals from the catalogue and, for
// drafts, the list of charges confirming would create.
func (h *Handler) fillTotals(ctx context.Context, q db.Querier, e *Enrolment) error {
	cd, err := loadCalendar(ctx, q, e.EntityID, e.SchoolYearID)
	if err != nil {
		return err
	}
	var active []uuid.UUID
	for _, s := range e.Services {
		if s.ToMonth == nil {
			active = append(active, s.FeeItemID)
		}
	}
	fees, err := loadFees(ctx, q, e.EntityID, append(active, e.OneOffFeeIDs...))
	if err != nil {
		return err
	}
	drafts, err := billing.Generate(cd.cal, e.StartMonth, fees, cd.vatMethod, h.today())
	if err != nil {
		return err
	}
	months := map[int]bool{}
	for _, d := range drafts {
		if d.Kind == "monthly" {
			e.Totals.AnnualGross += d.Lines.Gross
			months[d.PeriodMonth] = true
		} else {
			e.Totals.OneOffGross += d.Lines.Gross
		}
	}
	e.Totals.Months = len(months)
	for _, f := range fees {
		if f.Kind == "monthly" {
			l, err := money.SplitVAT(f.Amount, f.VATMode, f.VATRateBP, cd.vatMethod)
			if err != nil {
				return err
			}
			e.Totals.MonthlyGross += l.Gross
			e.Totals.MonthlyNet += l.Net
			e.Totals.MonthlyVAT += l.VAT
		}
	}
	e.Preview = []PreviewCharge{}
	if e.Status == "draft" {
		for _, d := range drafts {
			e.Preview = append(e.Preview, PreviewCharge{d.Description, d.Kind, d.PeriodMonth, d.DueDate.Format(time.DateOnly), d.Lines})
		}
	}
	return nil
}

func loadFees(ctx context.Context, q db.Querier, entityID uuid.UUID, ids []uuid.UUID) ([]billing.Fee, error) {
	rows, err := q.Query(ctx, `
		SELECT id, name, kind, amount, vat_mode, vat_rate_bp, due_rule
		FROM fee_item WHERE entity_id = $1 AND id = ANY($2) ORDER BY kind DESC, mandatory DESC, code`, entityID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []billing.Fee
	for rows.Next() {
		var f billing.Fee
		var due []byte
		if err := rows.Scan(&f.ID, &f.Name, &f.Kind, &f.Amount, &f.VATMode, &f.VATRateBP, &due); err != nil {
			return nil, err
		}
		if f.Kind == "one_off" {
			if f.Due, err = billing.ParseDueRule(due, "one_off"); err != nil {
				return nil, err
			}
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Draft create / update
// ---------------------------------------------------------------------------

type enrolmentRequest struct {
	PupilID       uuid.UUID   `json:"pupil_id"`
	SchoolYearID  uuid.UUID   `json:"school_year_id"`
	ClassID       *uuid.UUID  `json:"class_id"`
	Type          string      `json:"type"`
	StartMonth    int         `json:"start_month"`
	MonthlyFeeIDs []uuid.UUID `json:"monthly_fee_ids"` // optional services; mandatory ones for the class are added
	OneOffFeeIDs  []uuid.UUID `json:"one_off_fee_ids"`
	Consents      Consents    `json:"consents"`
}

type applicableFee struct {
	ID        uuid.UUID
	Kind      string
	Mandatory bool
}

// applicableFees lists active catalogue items that apply to a class.
func applicableFees(ctx context.Context, q db.Querier, entityID, yearID, classID uuid.UUID) (map[uuid.UUID]applicableFee, error) {
	rows, err := q.Query(ctx, `
		SELECT f.id, f.kind, f.mandatory FROM fee_item f
		WHERE f.entity_id = $1 AND f.school_year_id = $2 AND f.active
		  AND (NOT EXISTS (SELECT 1 FROM fee_item_class fc WHERE fc.fee_item_id = f.id)
		       OR EXISTS (SELECT 1 FROM fee_item_class fc WHERE fc.fee_item_id = f.id AND fc.class_id = $3))`,
		entityID, yearID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]applicableFee{}
	for rows.Next() {
		var f applicableFee
		if err := rows.Scan(&f.ID, &f.Kind, &f.Mandatory); err != nil {
			return nil, err
		}
		out[f.ID] = f
	}
	return out, rows.Err()
}

// validate checks the request and returns the monthly services to contract
// (chosen + mandatory).
func (req *enrolmentRequest) validate(ctx context.Context, q db.Querier, entityID uuid.UUID, cal billing.Calendar) ([]uuid.UUID, error) {
	f := httpx.Fields{}
	f.Add(req.Type != "new" && req.Type != "returning", "type", "Tipo inválido.")
	if req.StartMonth == 0 && len(cal.BillingMonths) > 0 {
		req.StartMonth = cal.BillingMonths[0]
	}
	f.Add(!slices.Contains(cal.BillingMonths, req.StartMonth), "start_month", "Mês de início fora do calendário de cobrança.")
	var pupilOK bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pupil WHERE id = $1 AND status = 'active')`, req.PupilID).Scan(&pupilOK); err != nil {
		return nil, err
	}
	f.Add(!pupilOK, "pupil_id", "Aluno inválido ou inactivo.")
	if req.ClassID == nil {
		f.Add(len(req.MonthlyFeeIDs)+len(req.OneOffFeeIDs) > 0, "class_id", "Escolha a turma antes dos serviços.")
		return nil, f.Err()
	}
	var classOK bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM class WHERE id = $1 AND entity_id = $2 AND school_year_id = $3)`,
		req.ClassID, entityID, req.SchoolYearID).Scan(&classOK); err != nil {
		return nil, err
	}
	if !classOK {
		f.Add(true, "class_id", "Turma inválida para esta empresa e ano.")
		return nil, f.Err()
	}
	fees, err := applicableFees(ctx, q, entityID, req.SchoolYearID, *req.ClassID)
	if err != nil {
		return nil, err
	}
	monthly := []uuid.UUID{}
	for _, id := range req.MonthlyFeeIDs {
		fee, ok := fees[id]
		f.Add(!ok || fee.Kind != "monthly", "monthly_fee_ids", "Serviço mensal inválido para esta turma.")
		if ok && !slices.Contains(monthly, id) {
			monthly = append(monthly, id)
		}
	}
	hasMandatory := false
	for id, fee := range fees {
		if fee.Kind == "monthly" && fee.Mandatory {
			hasMandatory = true
			if !slices.Contains(monthly, id) {
				monthly = append(monthly, id)
			}
		}
	}
	f.Add(!hasMandatory, "class_id", "A turma não tem mensalidade obrigatória na tabela de preços.")
	seen := map[uuid.UUID]bool{}
	var oneOffs []uuid.UUID
	for _, id := range req.OneOffFeeIDs {
		fee, ok := fees[id]
		f.Add(!ok || fee.Kind != "one_off", "one_off_fee_ids", "Taxa inválida para esta turma.")
		if ok && !seen[id] {
			seen[id] = true
			oneOffs = append(oneOffs, id)
		}
	}
	req.OneOffFeeIDs = oneOffs
	return monthly, f.Err()
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req enrolmentRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out *Enrolment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		id, err := h.createDraft(r.Context(), tx, entityID, req)
		if err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, entityID, id, false)
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) createDraft(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, req enrolmentRequest) (uuid.UUID, error) {
	cd, err := loadCalendar(ctx, tx, entityID, req.SchoolYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, httpx.Validation(httpx.Fields{"school_year_id": "Ano lectivo inválido."})
	}
	if err != nil {
		return uuid.Nil, err
	}
	monthly, err := req.validate(ctx, tx, entityID, cd.cal)
	if err != nil {
		return uuid.Nil, err
	}
	consents, _ := json.Marshal(req.Consents)
	draft, _ := json.Marshal(draftData{OneOffFeeIDs: req.OneOffFeeIDs})
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO enrolment (entity_id, school_year_id, pupil_id, class_id, type, start_month, consents, draft_data, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		entityID, req.SchoolYearID, req.PupilID, req.ClassID, req.Type, req.StartMonth, consents, draft,
		auth.CurrentUser(ctx).ID).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	if err := setServices(ctx, tx, id, monthly, req.StartMonth); err != nil {
		return uuid.Nil, err
	}
	eid := entityID
	return id, audit.Write(ctx, tx, audit.Entry{EntityID: &eid, Action: "create", Table: "enrolment", RecordID: id.String(), After: req})
}

func setServices(ctx context.Context, tx pgx.Tx, enrolmentID uuid.UUID, feeIDs []uuid.UUID, from int) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contracted_service WHERE enrolment_id = $1`, enrolmentID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO contracted_service (enrolment_id, fee_item_id, from_month) SELECT $1, unnest($2::uuid[]), $3`,
		enrolmentID, feeIDs, from)
	return err
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req enrolmentRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out *Enrolment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := h.load(r.Context(), tx, entityID, id, true)
		if err != nil {
			return err
		}
		if before.Status != "draft" {
			return httpx.Conflict("not_draft", "Só matrículas em rascunho podem ser editadas; use a alteração de serviços.")
		}
		req.PupilID, req.SchoolYearID = before.PupilID, before.SchoolYearID
		cd, err := loadCalendar(r.Context(), tx, entityID, req.SchoolYearID)
		if err != nil {
			return err
		}
		monthly, err := req.validate(r.Context(), tx, entityID, cd.cal)
		if err != nil {
			return err
		}
		consents, _ := json.Marshal(req.Consents)
		draft, _ := json.Marshal(draftData{OneOffFeeIDs: req.OneOffFeeIDs})
		if _, err := tx.Exec(r.Context(), `
			UPDATE enrolment SET class_id=$2, type=$3, start_month=$4, consents=$5, draft_data=$6 WHERE id=$1`,
			id, req.ClassID, req.Type, req.StartMonth, consents, draft); err != nil {
			return err
		}
		if err := setServices(r.Context(), tx, id, monthly, req.StartMonth); err != nil {
			return err
		}
		if out, err = h.load(r.Context(), tx, entityID, id, false); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "update", Table: "enrolment", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	s := auth.EntityScope(r.Context())
	var entityID uuid.UUID
	if err := h.Pool.QueryRow(r.Context(), `SELECT entity_id FROM enrolment WHERE id = $1 AND entity_id = ANY($2)`, id, s.EntityIDs).Scan(&entityID); err != nil {
		return httpx.NotFound()
	}
	e, err := h.load(r.Context(), h.Pool, entityID, id, false)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, e)
	return nil
}

func (h *Handler) deleteDraft(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		e, err := h.load(r.Context(), tx, entityID, id, true)
		if err != nil {
			return err
		}
		if e.Status != "draft" {
			return httpx.Conflict("not_draft", "Só rascunhos podem ser apagados; use a desistência.")
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM enrolment WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "delete", Table: "enrolment", RecordID: id.String(), Before: e})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
