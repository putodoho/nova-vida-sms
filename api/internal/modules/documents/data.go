package documents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// Entity header printed on every document.
type Entity struct {
	LegalName string `json:"legal_name"`
	ShortName string `json:"short_name"`
	NUIT      string `json:"nuit"`
	Address   string `json:"address"`
	Motto     string `json:"motto"`
}

type Account struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Holder string `json:"holder"`
	Number string `json:"number"`
	NIB    string `json:"nib"`
}

func loadEntity(ctx context.Context, q db.Querier, id uuid.UUID) (Entity, []Account, error) {
	var e Entity
	if err := q.QueryRow(ctx, `SELECT legal_name, short_name, nuit, address, motto FROM entity WHERE id = $1`, id).
		Scan(&e.LegalName, &e.ShortName, &e.NUIT, &e.Address, &e.Motto); err != nil {
		return e, nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT kind, label, holder, number, nib FROM receiving_account
		WHERE entity_id = $1 AND active AND kind <> 'cash' ORDER BY kind, label`, id)
	if err != nil {
		return e, nil, err
	}
	defer rows.Close()
	var accs []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.Kind, &a.Label, &a.Holder, &a.Number, &a.NIB); err != nil {
			return e, nil, err
		}
		accs = append(accs, a)
	}
	return e, accs, rows.Err()
}

var methodLabels = map[string]string{"cash": "Numerário", "bank": "Transferência / depósito bancário", "mpesa": "M-Pesa", "emola": "e-Mola"}

// ---------------------------------------------------------------------------
// Receipt
// ---------------------------------------------------------------------------

type ReceiptLine struct {
	Description string       `json:"description"`
	Amount      money.Amount `json:"amount"`
}

type Receipt struct {
	Entity       Entity        `json:"entity"`
	Number       string        `json:"number"`
	PaidOn       string        `json:"paid_on"`
	IssuedAt     string        `json:"issued_at"`
	PupilName    string        `json:"pupil_name"`
	PupilNumber  string        `json:"pupil_number"`
	ClassName    string        `json:"class_name"`
	PayerName    string        `json:"payer_name"`
	Amount       money.Amount  `json:"amount"`
	AmountWords  string        `json:"amount_words"`
	Method       string        `json:"method"`
	Reference    string        `json:"reference"`
	AccountLabel string        `json:"account_label"`
	Lines        []ReceiptLine `json:"lines"`
	Credit       money.Amount  `json:"credit"`
	Overdue      money.Amount  `json:"overdue"`
	Outstanding  money.Amount  `json:"outstanding"`
	ReceivedBy   string        `json:"received_by"`
	Cancelled    bool          `json:"cancelled"`
	CancelReason string        `json:"cancel_reason"`
}

func buildReceipt(ctx context.Context, q db.Querier, entityIDs []uuid.UUID, paymentID uuid.UUID, today time.Time) (*Receipt, error) {
	var r Receipt
	var entityID, pupilID uuid.UUID
	var paidOn, created time.Time
	var cancelledAt *time.Time
	var class, payer, account *string
	var allocated money.Amount
	err := q.QueryRow(ctx, `
		SELECT p.entity_id, p.pupil_id, p.receipt_no, p.paid_on, p.created_at, pu.full_name, pu.number,
		       (SELECT c.name FROM enrolment e JOIN class c ON c.id = e.class_id
		         WHERE e.pupil_id = p.pupil_id AND e.entity_id = p.entity_id AND e.status <> 'draft'
		         ORDER BY e.created_at DESC LIMIT 1),
		       (SELECT g.full_name FROM pupil_guardian pg JOIN guardian g ON g.id = pg.guardian_id WHERE pg.pupil_id = p.pupil_id AND pg.is_payer),
		       p.amount, p.method, p.reference, ra.label, u.display_name, p.cancelled_at, p.cancel_reason,
		       COALESCE((SELECT sum(a.amount) FROM allocation a WHERE a.payment_id = p.id AND a.active), 0)::bigint
		FROM payment p JOIN pupil pu ON pu.id = p.pupil_id JOIN app_user u ON u.id = p.received_by
		LEFT JOIN receiving_account ra ON ra.id = p.receiving_account_id
		WHERE p.id = $1 AND p.entity_id = ANY($2)`, paymentID, entityIDs).
		Scan(&entityID, &pupilID, &r.Number, &paidOn, &created, &r.PupilName, &r.PupilNumber, &class, &payer,
			&r.Amount, &r.Method, &r.Reference, &account, &r.ReceivedBy, &cancelledAt, &r.CancelReason, &allocated)
	if err != nil {
		return nil, httpx.NotFound()
	}
	r.PaidOn, r.IssuedAt = paidOn.Format("02/01/2006"), created.Format("02/01/2006 15:04")
	r.ClassName, r.PayerName, r.AccountLabel = deref(class), deref(payer), deref(account)
	r.Method = methodLabels[r.Method]
	r.AmountWords = r.Amount.Words()
	r.Cancelled = cancelledAt != nil
	if !r.Cancelled {
		r.Credit = r.Amount - allocated
	}
	if r.Entity, _, err = loadEntity(ctx, q, entityID); err != nil {
		return nil, err
	}
	// Lines: allocations as made (kept for cancelled receipts too).
	rows, err := q.Query(ctx, `
		SELECT c.description, a.amount FROM allocation a JOIN charge c ON c.id = a.charge_id
		WHERE a.payment_id = $1 ORDER BY c.due_date, c.kind`, paymentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l ReceiptLine
		if err := rows.Scan(&l.Description, &l.Amount); err != nil {
			rows.Close()
			return nil, err
		}
		r.Lines = append(r.Lines, l)
	}
	rows.Close()
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(remaining) FILTER (WHERE due_date < $3), 0)::bigint, COALESCE(sum(remaining), 0)::bigint
		FROM v_charge_balance WHERE entity_id = $1 AND pupil_id = $2 AND NOT cancelled`, entityID, pupilID, today).
		Scan(&r.Overdue, &r.Outstanding); err != nil {
		return nil, err
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Annual statement
// ---------------------------------------------------------------------------

type ServiceLine struct {
	Name         string       `json:"name"`
	MonthlyGross money.Amount `json:"monthly_gross"`
	MonthlyVAT   money.Amount `json:"monthly_vat"`
	MonthlyNet   money.Amount `json:"monthly_net"`
	Months       int          `json:"months"`
	AnnualGross  money.Amount `json:"annual_gross"`
	AnnualVAT    money.Amount `json:"annual_vat"`
	AnnualNet    money.Amount `json:"annual_net"`
}

type OneOffLine struct {
	Description string       `json:"description"`
	DueDate     string       `json:"due_date"`
	Amount      money.Amount `json:"amount"`
	Paid        money.Amount `json:"paid"`
	Remaining   money.Amount `json:"remaining"`
}

type MonthLine struct {
	N         int          `json:"n"`
	Month     string       `json:"month"`
	DueDate   string       `json:"due_date"`
	Due       money.Amount `json:"due"`
	Penalty   money.Amount `json:"penalty"`
	Discount  money.Amount `json:"discount"`
	Paid      money.Amount `json:"paid"`
	Remaining money.Amount `json:"remaining"`
	PaidOn    string       `json:"paid_on"`
	Receipts  string       `json:"receipts"`
	Status    string       `json:"status"` // paid | overdue | upcoming | partial
}

type Statement struct {
	Entity       Entity        `json:"entity"`
	Number       string        `json:"number"`
	IssuedOn     string        `json:"issued_on"`
	Year         int           `json:"year"`
	PupilName    string        `json:"pupil_name"`
	PupilNumber  string        `json:"pupil_number"`
	ClassName    string        `json:"class_name"`
	PayerName    string        `json:"payer_name"`
	PayerPhone   string        `json:"payer_phone"`
	Services     []ServiceLine `json:"services"`
	ServiceTotal ServiceLine   `json:"service_total"`
	HasVAT       bool          `json:"has_vat"`
	VATRate      string        `json:"vat_rate"`
	OneOffs      []OneOffLine  `json:"one_offs"`
	Months       []MonthLine   `json:"months"`
	MonthTotal   MonthLine     `json:"month_total"`
	Total        money.Amount  `json:"total"`
	Paid         money.Amount  `json:"paid"`
	Outstanding  money.Amount  `json:"outstanding"`
	Overdue      money.Amount  `json:"overdue"`
	Credit       money.Amount  `json:"credit"`
	DueDay       int           `json:"due_day"`
	PenaltyText  string        `json:"penalty_text"`
	Accounts     []Account     `json:"accounts"`
}

func penaltyText(raw []byte) string {
	var r billing.PenaltyRule
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	switch r.Type {
	case billing.PenaltyPercent:
		return fmt.Sprintf("%s%% sobre o valor mensal em atraso", strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", float64(r.RateBP)/100), "0"), ".0"))
	case billing.PenaltyFixedTiers:
		var parts []string
		for _, t := range r.Tiers {
			parts = append(parts, fmt.Sprintf("%s a partir do dia %d", t.Amount.Format(), t.AfterDay+1))
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

// buildStatement assembles the annual statement of a confirmed or withdrawn
// enrolment as it stands today.
func buildStatement(ctx context.Context, q db.Querier, enrolmentID uuid.UUID, number string, today time.Time) (*Statement, error) {
	s := &Statement{Number: number, IssuedOn: today.Format("02/01/2006")}
	var entityID, pupilID uuid.UUID
	var class, payer, phone *string
	var rule []byte
	err := q.QueryRow(ctx, `
		SELECT e.entity_id, e.pupil_id, sy.year, sy.due_day, p.full_name, p.number, c.name, g.full_name, g.phone1, l.penalty_rule
		FROM enrolment e
		JOIN school_year sy ON sy.id = e.school_year_id
		JOIN pupil p ON p.id = e.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		LEFT JOIN level l ON l.id = c.level_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE e.id = $1`, enrolmentID).
		Scan(&entityID, &pupilID, &s.Year, &s.DueDay, &s.PupilName, &s.PupilNumber, &class, &payer, &phone, &rule)
	if err != nil {
		return nil, httpx.NotFound()
	}
	s.ClassName, s.PayerName, s.PayerPhone = deref(class), deref(payer), deref(phone)
	s.PenaltyText = penaltyText(rule)
	if s.Entity, s.Accounts, err = loadEntity(ctx, q, entityID); err != nil {
		return nil, err
	}

	type row struct {
		id          uuid.UUID
		kind        string
		fee         *uuid.UUID
		feeName     *string
		period      *int
		parent      *uuid.UUID
		parentMonth *int
		desc        string
		due         time.Time
		gross, vat  money.Amount
		rate        int
		discount    money.Amount
		paid        money.Amount
		remaining   money.Amount
		payable     money.Amount
	}
	rows, err := q.Query(ctx, `
		SELECT c.id, c.kind, c.fee_item_id, f.name, c.period_month, c.parent_charge_id, pc.period_month, c.description, c.due_date,
		       c.amount_gross, c.amount_vat, c.vat_rate_bp, b.discount, b.paid, b.remaining, b.payable
		FROM charge c
		JOIN v_charge_balance b ON b.charge_id = c.id
		LEFT JOIN fee_item f ON f.id = c.fee_item_id
		LEFT JOIN charge pc ON pc.id = c.parent_charge_id
		WHERE c.enrolment_id = $1 AND c.cancelled_at IS NULL
		ORDER BY c.due_date, c.kind`, enrolmentID)
	if err != nil {
		return nil, err
	}
	var all []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.kind, &x.fee, &x.feeName, &x.period, &x.parent, &x.parentMonth, &x.desc, &x.due,
			&x.gross, &x.vat, &x.rate, &x.discount, &x.paid, &x.remaining, &x.payable); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Receipts and last payment date per charge.
	type payInfo struct {
		last     time.Time
		receipts []string
	}
	pays := map[uuid.UUID]*payInfo{}
	prow, err := q.Query(ctx, `
		SELECT a.charge_id, p.paid_on, p.receipt_no FROM allocation a
		JOIN payment p ON p.id = a.payment_id JOIN charge c ON c.id = a.charge_id
		WHERE c.enrolment_id = $1 AND a.active ORDER BY p.paid_on`, enrolmentID)
	if err != nil {
		return nil, err
	}
	for prow.Next() {
		var cid uuid.UUID
		var on time.Time
		var rec string
		if err := prow.Scan(&cid, &on, &rec); err != nil {
			prow.Close()
			return nil, err
		}
		pi := pays[cid]
		if pi == nil {
			pi = &payInfo{}
			pays[cid] = pi
		}
		pi.last = on
		pi.receipts = append(pi.receipts, rec)
	}
	prow.Close()

	services := map[uuid.UUID]*ServiceLine{}
	var serviceOrder []uuid.UUID
	months := map[int]*MonthLine{}
	monthReceipts := map[int]map[string]bool{}
	monthLast := map[int]time.Time{}
	monthDue := map[int]time.Time{}
	for _, x := range all {
		s.Total += x.payable
		s.Paid += x.paid
		s.Outstanding += x.remaining
		if x.remaining > 0 && x.due.Before(today) {
			s.Overdue += x.remaining
		}
		if x.vat > 0 {
			s.HasVAT = true
			s.VATRate = fmt.Sprintf("%d%%", x.rate/100)
		}
		month := 0
		switch x.kind {
		case "monthly":
			month = *x.period
			sl := services[*x.fee]
			if sl == nil {
				sl = &ServiceLine{Name: deref(x.feeName), MonthlyGross: x.gross, MonthlyVAT: x.vat, MonthlyNet: x.gross - x.vat}
				services[*x.fee] = sl
				serviceOrder = append(serviceOrder, *x.fee)
			}
			sl.Months++
			sl.AnnualGross += x.gross
			sl.AnnualVAT += x.vat
			sl.AnnualNet += x.gross - x.vat
		case "penalty":
			if x.parentMonth != nil {
				month = *x.parentMonth
			}
		case "one_off":
			s.OneOffs = append(s.OneOffs, OneOffLine{x.desc, x.due.Format("02/01/2006"), x.payable, x.paid, x.remaining})
			continue
		}
		if month == 0 {
			continue
		}
		ml := months[month]
		if ml == nil {
			ml = &MonthLine{Month: billing.MonthNames[month]}
			months[month] = ml
			monthReceipts[month] = map[string]bool{}
		}
		if x.kind == "monthly" {
			ml.Due += x.gross
			monthDue[month] = x.due
		} else {
			ml.Penalty += x.gross
		}
		ml.Discount += x.discount
		ml.Paid += x.paid
		ml.Remaining += x.remaining
		if pi := pays[x.id]; pi != nil {
			for _, rc := range pi.receipts {
				monthReceipts[month][rc] = true
			}
			if pi.last.After(monthLast[month]) {
				monthLast[month] = pi.last
			}
		}
	}
	for _, id := range serviceOrder {
		sl := services[id]
		s.Services = append(s.Services, *sl)
		t := &s.ServiceTotal
		t.MonthlyGross += sl.MonthlyGross
		t.MonthlyVAT += sl.MonthlyVAT
		t.MonthlyNet += sl.MonthlyNet
		t.AnnualGross += sl.AnnualGross
		t.AnnualVAT += sl.AnnualVAT
		t.AnnualNet += sl.AnnualNet
		t.Months = max(t.Months, sl.Months)
	}
	keys := make([]int, 0, len(months))
	for m := range months {
		keys = append(keys, m)
	}
	sort.Ints(keys)
	for i, m := range keys {
		ml := months[m]
		ml.N = i + 1
		if d, ok := monthDue[m]; ok {
			ml.DueDate = d.Format("02/01/2006")
			ml.Status = billing.Status(false, ml.Due+ml.Penalty-ml.Discount, ml.Paid, d, today)
		}
		if t, ok := monthLast[m]; ok {
			ml.PaidOn = t.Format("02/01/2006")
		}
		var recs []string
		for rc := range monthReceipts[m] {
			recs = append(recs, rc)
		}
		sort.Strings(recs)
		ml.Receipts = strings.Join(recs, ", ")
		s.Months = append(s.Months, *ml)
		t := &s.MonthTotal
		t.Due += ml.Due
		t.Penalty += ml.Penalty
		t.Discount += ml.Discount
		t.Paid += ml.Paid
		t.Remaining += ml.Remaining
	}
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(p.amount - COALESCE((SELECT sum(a.amount) FROM allocation a WHERE a.payment_id = p.id AND a.active), 0)), 0)::bigint
		FROM payment p WHERE p.entity_id = $1 AND p.pupil_id = $2 AND p.cancelled_at IS NULL`, entityID, pupilID).Scan(&s.Credit); err != nil {
		return nil, err
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Enrolment form
// ---------------------------------------------------------------------------

type Person struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
	Extra        string `json:"extra"`
}

type FeeLine struct {
	Name   string       `json:"name"`
	Amount money.Amount `json:"amount"`
}

type EnrolmentForm struct {
	Entity        Entity       `json:"entity"`
	Year          int          `json:"year"`
	Status        string       `json:"status"`
	Type          string       `json:"type"`
	PupilName     string       `json:"pupil_name"`
	PupilNumber   string       `json:"pupil_number"`
	Sex           string       `json:"sex"`
	BirthDate     string       `json:"birth_date"`
	Neighbourhood string       `json:"neighbourhood"`
	Religion      string       `json:"religion"`
	Orphan        string       `json:"orphan"`
	ClassName     string       `json:"class_name"`
	StartMonth    string       `json:"start_month"`
	ShowHealth    bool         `json:"show_health"`
	Chronic       string       `json:"chronic"`
	Allergies     string       `json:"allergies"`
	Diet          string       `json:"diet"`
	Medication    string       `json:"medication"`
	Doctor        string       `json:"doctor"`
	Guardians     []Person     `json:"guardians"`
	Pickup        []Person     `json:"pickup"`
	Emergency     []Person     `json:"emergency"`
	Monthly       []FeeLine    `json:"monthly"`
	MonthlyTotal  money.Amount `json:"monthly_total"`
	OneOffs       []FeeLine    `json:"one_offs"`
	OneOffTotal   money.Amount `json:"one_off_total"`
	Consents      struct {
		ImageUse, FeesAgreement, UniformBooks, EmergencyCare bool
	} `json:"consents"`
	PenaltyText string `json:"penalty_text"`
	DueDay      int    `json:"due_day"`
	PrintedOn   string `json:"printed_on"`
}

var relationshipLabels = map[string]string{"mother": "Mãe", "father": "Pai", "guardian": "Encarregado", "other": "Outro"}
var orphanLabels = map[string]string{"none": "Não", "father": "Órfão de pai", "mother": "Órfão de mãe", "both": "Órfão de pai e mãe"}

func buildEnrolmentForm(ctx context.Context, q db.Querier, entityID, enrolmentID uuid.UUID, showHealth bool, today time.Time) (*EnrolmentForm, error) {
	f := &EnrolmentForm{ShowHealth: showHealth, PrintedOn: today.Format("02/01/2006")}
	var pupilID uuid.UUID
	var class *string
	var birth time.Time
	var startMonth int
	var consents, draft, rule []byte
	err := q.QueryRow(ctx, `
		SELECT e.pupil_id, sy.year, sy.due_day, e.status, e.type, e.start_month, e.consents, e.draft_data,
		       p.full_name, p.number, p.sex, p.birth_date, p.neighbourhood, p.religion, p.orphan, c.name, l.penalty_rule
		FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id JOIN pupil p ON p.id = e.pupil_id
		LEFT JOIN class c ON c.id = e.class_id LEFT JOIN level l ON l.id = c.level_id
		WHERE e.id = $1 AND e.entity_id = $2`, enrolmentID, entityID).
		Scan(&pupilID, &f.Year, &f.DueDay, &f.Status, &f.Type, &startMonth, &consents, &draft,
			&f.PupilName, &f.PupilNumber, &f.Sex, &birth, &f.Neighbourhood, &f.Religion, &f.Orphan, &class, &rule)
	if err != nil {
		return nil, httpx.NotFound()
	}
	f.BirthDate = birth.Format("02/01/2006")
	f.ClassName = deref(class)
	f.StartMonth = billing.MonthNames[startMonth]
	f.Orphan = orphanLabels[f.Orphan]
	f.PenaltyText = penaltyText(rule)
	var c struct {
		ImageUse      bool `json:"image_use"`
		FeesAgreement bool `json:"fees_agreement"`
		UniformBooks  bool `json:"uniform_books"`
		EmergencyCare bool `json:"emergency_care"`
	}
	_ = json.Unmarshal(consents, &c)
	f.Consents.ImageUse, f.Consents.FeesAgreement, f.Consents.UniformBooks, f.Consents.EmergencyCare = c.ImageUse, c.FeesAgreement, c.UniformBooks, c.EmergencyCare
	if f.Entity, _, err = loadEntity(ctx, q, entityID); err != nil {
		return nil, err
	}
	if showHealth {
		_ = q.QueryRow(ctx, `SELECT chronic_condition, allergies, diet_restrictions, medication,
			trim(doctor_name || ' ' || doctor_phone) FROM health_record WHERE pupil_id = $1`, pupilID).
			Scan(&f.Chronic, &f.Allergies, &f.Diet, &f.Medication, &f.Doctor)
	}
	rows, err := q.Query(ctx, `
		SELECT g.full_name, pg.relationship, trim(g.phone1 || ' ' || g.phone2),
		       concat_ws(' · ', CASE WHEN pg.is_payer THEN 'Responsável pelos pagamentos' END,
		                        CASE WHEN pg.receives_notices THEN 'Recebe comunicações' END,
		                        CASE WHEN pg.may_pick_up THEN 'Pode buscar a criança' END)
		FROM pupil_guardian pg JOIN guardian g ON g.id = pg.guardian_id
		WHERE pg.pupil_id = $1 ORDER BY pg.is_payer DESC`, pupilID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.Name, &p.Relationship, &p.Phone, &p.Extra); err != nil {
			rows.Close()
			return nil, err
		}
		p.Relationship = relationshipLabels[p.Relationship]
		f.Guardians = append(f.Guardians, p)
	}
	rows.Close()
	if f.Pickup, err = people(ctx, q, `SELECT full_name, '', phone, id_doc FROM pickup_person WHERE pupil_id = $1 ORDER BY full_name`, pupilID); err != nil {
		return nil, err
	}
	if f.Emergency, err = people(ctx, q, `SELECT full_name, relationship, phone, '' FROM emergency_contact WHERE pupil_id = $1 ORDER BY position`, pupilID); err != nil {
		return nil, err
	}
	// Services: contracted monthly services and one-off items (as chosen at enrolment).
	var d struct {
		OneOffFeeIDs []uuid.UUID `json:"one_off_fee_ids"`
	}
	_ = json.Unmarshal(draft, &d)
	rows, err = q.Query(ctx, `
		SELECT f.name, f.amount, f.kind FROM fee_item f
		WHERE f.id IN (SELECT fee_item_id FROM contracted_service WHERE enrolment_id = $1 AND to_month IS NULL) OR f.id = ANY($2)
		ORDER BY f.kind DESC, f.mandatory DESC, f.name`, enrolmentID, d.OneOffFeeIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l FeeLine
		var kind string
		if err := rows.Scan(&l.Name, &l.Amount, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "monthly" {
			f.Monthly = append(f.Monthly, l)
			f.MonthlyTotal += l.Amount
		} else {
			f.OneOffs = append(f.OneOffs, l)
			f.OneOffTotal += l.Amount
		}
	}
	rows.Close()
	return f, rows.Err()
}

func people(ctx context.Context, q db.Querier, sql string, id uuid.UUID) ([]Person, error) {
	rows, err := q.Query(ctx, sql, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.Name, &p.Relationship, &p.Phone, &p.Extra); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
