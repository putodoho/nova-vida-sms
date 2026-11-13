// Package reports serves the dashboard and the management reports. Every
// report is a report.Table, returned as JSON or as .xlsx (?format=xlsx).
package reports

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
	"github.com/putodoho/nova-vida-sms/api/internal/report"
)

type Handler struct {
	Pool *pgxpool.Pool
	Loc  *time.Location
	Now  func() time.Time
}

func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.ReportsRead))
		r.Method(http.MethodGet, "/reports/dashboard", httpx.Handler(h.dashboard))
		r.Method(http.MethodGet, "/reports/income", httpx.Handler(h.income))
		r.Method(http.MethodGet, "/reports/arrears", httpx.Handler(h.arrears))
		r.Method(http.MethodGet, "/reports/pupils", httpx.Handler(h.pupils))
		r.Method(http.MethodGet, "/reports/discounts", httpx.Handler(h.discounts))
		r.Method(http.MethodGet, "/reports/penalties", httpx.Handler(h.penalties))
		r.Method(http.MethodGet, "/reports/vat", httpx.Handler(h.vat))
	})
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

type yearInfo struct {
	id     uuid.UUID
	year   int
	months []int
}

func (h *Handler) year(ctx context.Context, r *http.Request) (yearInfo, error) {
	var y yearInfo
	var months []int32
	err := h.Pool.QueryRow(ctx, `
		SELECT id, year, billing_months FROM school_year
		WHERE ($1 = 0 AND id = (SELECT id FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1)) OR year = $1`,
		httpx.QueryInt(r, "year", 0)).Scan(&y.id, &y.year, &months)
	if err != nil {
		return y, httpx.NotFound()
	}
	for _, m := range months {
		y.months = append(y.months, int(m))
	}
	return y, nil
}

func scope(r *http.Request) []uuid.UUID { return auth.EntityScope(r.Context()).EntityIDs }

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

type MonthPoint struct {
	Month    int          `json:"month"`
	Label    string       `json:"label"`
	Expected money.Amount `json:"expected"`
	Received money.Amount `json:"received"`
}

type OverdueAccount struct {
	PupilID   uuid.UUID    `json:"pupil_id"`
	Name      string       `json:"name"`
	ClassName *string      `json:"class_name"`
	Overdue   money.Amount `json:"overdue"`
	Days      int          `json:"days"`
	Phone     *string      `json:"phone"`
}

type RecentPayment struct {
	ID        uuid.UUID    `json:"id"`
	ReceiptNo string       `json:"receipt_no"`
	PupilName string       `json:"pupil_name"`
	Amount    money.Amount `json:"amount"`
	Method    string       `json:"method"`
	PaidOn    string       `json:"paid_on"`
}

type Dashboard struct {
	Today            string                  `json:"today"`
	Year             int                     `json:"year"`
	ExpectedMonth    money.Amount            `json:"expected_month"`
	ReceivedMonth    money.Amount            `json:"received_month"`
	CollectionRate   float64                 `json:"collection_rate"` // paid / payable of charges due so far this year
	Overdue          money.Amount            `json:"overdue"`
	PupilsOverdue    int                     `json:"pupils_overdue"`
	ActivePupils     int                     `json:"active_pupils"`
	Monthly          []MonthPoint            `json:"monthly"`
	ByMethodMonth    map[string]money.Amount `json:"by_method_month"`
	TopOverdue       []OverdueAccount        `json:"top_overdue"`
	RecentPayments   []RecentPayment         `json:"recent_payments"`
	PendingDiscounts int                     `json:"pending_discounts"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	y, err := h.year(ctx, r)
	if err != nil {
		return err
	}
	today := h.today()
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	ents := scope(r)
	d := Dashboard{Today: today.Format(time.DateOnly), Year: y.year, ByMethodMonth: map[string]money.Amount{},
		TopOverdue: []OverdueAccount{}, RecentPayments: []RecentPayment{}}

	var payableDue, paidDue money.Amount
	if err := h.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(b.payable) FILTER (WHERE b.kind <> 'penalty' AND b.due_date >= $3 AND b.due_date < $4), 0)::bigint,
		       COALESCE(sum(b.remaining) FILTER (WHERE b.due_date < $5), 0)::bigint,
		       count(DISTINCT b.pupil_id) FILTER (WHERE b.due_date < $5 AND b.remaining > 0)::int,
		       COALESCE(sum(b.payable) FILTER (WHERE b.due_date <= $5), 0)::bigint,
		       COALESCE(sum(b.paid) FILTER (WHERE b.due_date <= $5), 0)::bigint
		FROM v_charge_balance b JOIN enrolment e ON e.id = b.enrolment_id
		WHERE b.entity_id = ANY($1) AND e.school_year_id = $2 AND NOT b.cancelled`,
		ents, y.id, monthStart, monthEnd, today).Scan(&d.ExpectedMonth, &d.Overdue, &d.PupilsOverdue, &payableDue, &paidDue); err != nil {
		return err
	}
	if payableDue > 0 {
		d.CollectionRate = float64(paidDue) / float64(payableDue)
	}
	if err := h.Pool.QueryRow(ctx, `
		SELECT count(*)::int FROM enrolment WHERE entity_id = ANY($1) AND school_year_id = $2 AND status = 'confirmed'`,
		ents, y.id).Scan(&d.ActivePupils); err != nil {
		return err
	}
	if err := h.Pool.QueryRow(ctx, `SELECT count(*)::int FROM discount_request WHERE entity_id = ANY($1) AND status = 'pending'`,
		ents).Scan(&d.PendingDiscounts); err != nil {
		return err
	}

	// Expected (by due month) and received (by payment month) for the billing calendar.
	expected := map[int]money.Amount{}
	received := map[int]money.Amount{}
	rows, err := h.Pool.Query(ctx, `
		SELECT extract(month FROM b.due_date)::int, sum(b.payable)::bigint
		FROM v_charge_balance b JOIN enrolment e ON e.id = b.enrolment_id
		WHERE b.entity_id = ANY($1) AND e.school_year_id = $2 AND NOT b.cancelled AND b.kind <> 'penalty'
		  AND extract(year FROM b.due_date) = $3
		GROUP BY 1`, ents, y.id, y.year)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m int
		var v money.Amount
		if err := rows.Scan(&m, &v); err != nil {
			rows.Close()
			return err
		}
		expected[m] = v
	}
	rows.Close()
	rows, err = h.Pool.Query(ctx, `
		SELECT extract(month FROM paid_on)::int, sum(amount)::bigint FROM payment
		WHERE entity_id = ANY($1) AND cancelled_at IS NULL AND extract(year FROM paid_on) = $2 GROUP BY 1`, ents, y.year)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m int
		var v money.Amount
		if err := rows.Scan(&m, &v); err != nil {
			rows.Close()
			return err
		}
		received[m] = v
	}
	rows.Close()
	for _, m := range y.months {
		d.Monthly = append(d.Monthly, MonthPoint{m, billing.MonthNames[m], expected[m], received[m]})
	}

	rows, err = h.Pool.Query(ctx, `
		SELECT method, sum(amount)::bigint FROM payment
		WHERE entity_id = ANY($1) AND cancelled_at IS NULL AND paid_on >= $2 AND paid_on < $3 GROUP BY method`, ents, monthStart, monthEnd)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m string
		var v money.Amount
		if err := rows.Scan(&m, &v); err != nil {
			rows.Close()
			return err
		}
		d.ByMethodMonth[m] = v
		d.ReceivedMonth += v
	}
	rows.Close()

	rows, err = h.Pool.Query(ctx, `
		SELECT p.id, p.full_name, c.name, sum(b.remaining)::bigint, ($3::date - min(b.due_date))::int, g.phone1
		FROM v_charge_balance b
		JOIN enrolment e ON e.id = b.enrolment_id
		JOIN pupil p ON p.id = b.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE b.entity_id = ANY($1) AND e.school_year_id = $2 AND NOT b.cancelled AND b.remaining > 0 AND b.due_date < $3
		GROUP BY p.id, c.name, g.phone1
		ORDER BY 4 DESC LIMIT 10`, ents, y.id, today)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a OverdueAccount
		if err := rows.Scan(&a.PupilID, &a.Name, &a.ClassName, &a.Overdue, &a.Days, &a.Phone); err != nil {
			rows.Close()
			return err
		}
		d.TopOverdue = append(d.TopOverdue, a)
	}
	rows.Close()

	rows, err = h.Pool.Query(ctx, `
		SELECT p.id, p.receipt_no, pu.full_name, p.amount, p.method, p.paid_on
		FROM payment p JOIN pupil pu ON pu.id = p.pupil_id
		WHERE p.entity_id = ANY($1) AND p.cancelled_at IS NULL
		ORDER BY p.created_at DESC LIMIT 8`, ents)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p RecentPayment
		var on time.Time
		if err := rows.Scan(&p.ID, &p.ReceiptNo, &p.PupilName, &p.Amount, &p.Method, &on); err != nil {
			rows.Close()
			return err
		}
		p.PaidOn = on.Format(time.DateOnly)
		d.RecentPayments = append(d.RecentPayments, p)
	}
	rows.Close()
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------

var methodLabels = map[string]string{"cash": "Numerário", "bank": "Banco", "mpesa": "M-Pesa", "emola": "e-Mola"}

func dateRange(r *http.Request, def func() (time.Time, time.Time)) (time.Time, time.Time, error) {
	from, to := def()
	if v := r.URL.Query().Get("from"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return from, to, httpx.BadRequest("from inválido (AAAA-MM-DD).")
		}
		from = d
	}
	if v := r.URL.Query().Get("to"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return from, to, httpx.BadRequest("to inválido (AAAA-MM-DD).")
		}
		to = d
	}
	return from, to, nil
}

// income: valid payments between from and to (default: this calendar year),
// grouped by month, method or account.
func (h *Handler) income(w http.ResponseWriter, r *http.Request) error {
	today := h.today()
	from, to, err := dateRange(r, func() (time.Time, time.Time) {
		return time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC), time.Date(today.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		return err
	}
	group := r.URL.Query().Get("group")
	var keyExpr, label string
	switch group {
	case "", "month":
		group, keyExpr, label = "month", "to_char(p.paid_on, 'YYYY-MM')", "Mês"
	case "method":
		keyExpr, label = "p.method", "Método"
	case "account":
		keyExpr, label = "COALESCE(ra.label, '—')", "Conta"
	default:
		return httpx.BadRequest("group: month, method ou account.")
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT `+keyExpr+`, count(*)::int, sum(p.amount)::bigint
		FROM payment p LEFT JOIN receiving_account ra ON ra.id = p.receiving_account_id
		WHERE p.entity_id = ANY($1) AND p.cancelled_at IS NULL AND p.paid_on BETWEEN $2 AND $3
		GROUP BY 1 ORDER BY 1`, scope(r), from, to)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title:   fmt.Sprintf("Receitas de %s a %s", from.Format("02/01/2006"), to.Format("02/01/2006")),
		Columns: []report.Column{{Key: "key", Label: label, Type: report.Text}, {Key: "count", Label: "Pagamentos", Type: report.Integer}, {Key: "amount", Label: "Valor recebido", Type: report.Money}},
	}
	var n int
	var total money.Amount
	for rows.Next() {
		var key string
		var c int
		var v money.Amount
		if err := rows.Scan(&key, &c, &v); err != nil {
			return err
		}
		switch group {
		case "month":
			if mt, err := time.Parse("2006-01", key); err == nil {
				key = fmt.Sprintf("%s %d", billing.MonthNames[mt.Month()], mt.Year())
			}
		case "method":
			key = methodLabels[key]
		}
		t.Rows = append(t.Rows, []any{key, c, v})
		n += c
		total += v
	}
	t.Totals = []any{"Total", n, total}
	return report.Write(w, r, t, "receitas")
}

// arrears: per pupil, what is overdue today and outstanding for the year.
func (h *Handler) arrears(w http.ResponseWriter, r *http.Request) error {
	y, err := h.year(r.Context(), r)
	if err != nil {
		return err
	}
	today := h.today()
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.number, p.full_name, c.name, g.full_name, g.phone1,
		       COALESCE(sum(b.remaining) FILTER (WHERE b.due_date < $3), 0)::bigint,
		       sum(b.remaining)::bigint,
		       COALESCE(($3::date - min(b.due_date) FILTER (WHERE b.due_date < $3 AND b.remaining > 0)), 0)::int
		FROM v_charge_balance b
		JOIN enrolment e ON e.id = b.enrolment_id
		JOIN pupil p ON p.id = b.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE b.entity_id = ANY($1) AND e.school_year_id = $2 AND NOT b.cancelled AND b.remaining > 0
		GROUP BY p.id, c.name, g.full_name, g.phone1
		HAVING sum(b.remaining) FILTER (WHERE b.due_date < $3) > 0
		ORDER BY 6 DESC`, scope(r), y.id, today)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title: fmt.Sprintf("Dívidas por aluno a %s", today.Format("02/01/2006")),
		Columns: []report.Column{
			{Key: "number", Label: "Nº", Type: report.Text}, {Key: "pupil", Label: "Aluno", Type: report.Text},
			{Key: "class", Label: "Turma", Type: report.Text}, {Key: "payer", Label: "Encarregado", Type: report.Text},
			{Key: "phone", Label: "Telefone", Type: report.Text}, {Key: "overdue", Label: "Em dívida", Type: report.Money},
			{Key: "outstanding", Label: "Total por pagar", Type: report.Money}, {Key: "days", Label: "Dias de atraso", Type: report.Integer},
		},
	}
	var to, ts money.Amount
	for rows.Next() {
		var num, name string
		var class, payer, phone *string
		var od, os money.Amount
		var days int
		if err := rows.Scan(&num, &name, &class, &payer, &phone, &od, &os, &days); err != nil {
			return err
		}
		t.Rows = append(t.Rows, []any{num, name, class, payer, phone, od, os, days})
		to += od
		ts += os
	}
	pupils := fmt.Sprintf("%d alunos", len(t.Rows))
	if len(t.Rows) == 1 {
		pupils = "1 aluno"
	}
	t.Totals = []any{"Total", pupils, nil, nil, nil, to, ts, nil}
	return report.Write(w, r, t, "dividas")
}

// pupils: enrolment numbers per class.
func (h *Handler) pupils(w http.ResponseWriter, r *http.Request) error {
	y, err := h.year(r.Context(), r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT en.short_name, c.name, c.capacity,
		       count(e.id) FILTER (WHERE e.status = 'confirmed')::int,
		       count(e.id) FILTER (WHERE e.status = 'confirmed' AND e.type = 'new')::int,
		       count(e.id) FILTER (WHERE e.status = 'confirmed' AND e.type = 'returning')::int,
		       count(e.id) FILTER (WHERE e.status = 'withdrawn')::int,
		       count(e.id) FILTER (WHERE e.status = 'draft')::int
		FROM class c JOIN entity en ON en.id = c.entity_id
		LEFT JOIN enrolment e ON e.class_id = c.id
		WHERE c.entity_id = ANY($1) AND c.school_year_id = $2
		GROUP BY en.short_name, c.id ORDER BY en.short_name, c.grade NULLS FIRST, c.name`, scope(r), y.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title: fmt.Sprintf("Alunos por turma: %d", y.year),
		Columns: []report.Column{
			{Key: "entity", Label: "Empresa", Type: report.Text}, {Key: "class", Label: "Turma", Type: report.Text},
			{Key: "capacity", Label: "Capacidade", Type: report.Integer}, {Key: "enrolled", Label: "Matriculados", Type: report.Integer},
			{Key: "new", Label: "Novos", Type: report.Integer}, {Key: "returning", Label: "Renovações", Type: report.Integer},
			{Key: "withdrawn", Label: "Desistências", Type: report.Integer}, {Key: "drafts", Label: "Rascunhos", Type: report.Integer},
			{Key: "occupancy", Label: "Ocupação", Type: report.Percent},
		},
	}
	sum := make([]int, 6)
	for rows.Next() {
		var ent, class string
		v := make([]int, 6)
		if err := rows.Scan(&ent, &class, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5]); err != nil {
			return err
		}
		occ := 0.0
		if v[0] > 0 {
			occ = float64(v[1]) / float64(v[0])
		}
		t.Rows = append(t.Rows, []any{ent, class, v[0], v[1], v[2], v[3], v[4], v[5], occ})
		for i := range v {
			sum[i] += v[i]
		}
	}
	occ := 0.0
	if sum[0] > 0 {
		occ = float64(sum[1]) / float64(sum[0])
	}
	t.Totals = []any{"Total", nil, sum[0], sum[1], sum[2], sum[3], sum[4], sum[5], occ}
	return report.Write(w, r, t, "alunos")
}

// discounts: approved discounts per pupil and type.
func (h *Handler) discounts(w http.ResponseWriter, r *http.Request) error {
	y, err := h.year(r.Context(), r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.full_name, c.name, dt.name, dr.reason, u.display_name, dr.decided_at::date, COALESCE(sum(d.amount), 0)::bigint
		FROM discount_request dr
		JOIN enrolment e ON e.id = dr.enrolment_id
		JOIN pupil p ON p.id = e.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		JOIN discount_type dt ON dt.id = dr.discount_type_id
		LEFT JOIN app_user u ON u.id = dr.decided_by
		LEFT JOIN discount d ON d.discount_request_id = dr.id
		LEFT JOIN charge ch ON ch.id = d.charge_id
		WHERE dr.entity_id = ANY($1) AND e.school_year_id = $2 AND dr.status = 'approved' AND (ch.id IS NULL OR ch.cancelled_at IS NULL)
		GROUP BY dr.id, p.full_name, c.name, dt.name, u.display_name ORDER BY p.full_name`, scope(r), y.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title: fmt.Sprintf("Descontos concedidos: %d", y.year),
		Columns: []report.Column{
			{Key: "pupil", Label: "Aluno", Type: report.Text}, {Key: "class", Label: "Turma", Type: report.Text},
			{Key: "type", Label: "Tipo", Type: report.Text}, {Key: "reason", Label: "Motivo", Type: report.Text},
			{Key: "approved_by", Label: "Aprovado por", Type: report.Text}, {Key: "approved_on", Label: "Data", Type: report.Date},
			{Key: "amount", Label: "Valor", Type: report.Money},
		},
	}
	var total money.Amount
	for rows.Next() {
		var pupil, typ, reason string
		var class, by *string
		var on *time.Time
		var amt money.Amount
		if err := rows.Scan(&pupil, &class, &typ, &reason, &by, &on, &amt); err != nil {
			return err
		}
		var date any
		if on != nil {
			date = *on
		}
		t.Rows = append(t.Rows, []any{pupil, class, typ, reason, by, date, amt})
		total += amt
	}
	t.Totals = []any{"Total", nil, nil, nil, nil, nil, total}
	return report.Write(w, r, t, "descontos")
}

// penalties: per month of the fee, penalties charged, paid, waived, open.
func (h *Handler) penalties(w http.ResponseWriter, r *http.Request) error {
	y, err := h.year(r.Context(), r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT COALESCE(c.period_month, extract(month FROM c.due_date)::int),
		       count(*)::int,
		       COALESCE(sum(c.amount_gross), 0)::bigint,
		       COALESCE(sum(b.paid) FILTER (WHERE NOT b.cancelled), 0)::bigint,
		       COALESCE(sum(c.amount_gross) FILTER (WHERE b.cancelled), 0)::bigint,
		       COALESCE(sum(b.remaining), 0)::bigint
		FROM charge c JOIN v_charge_balance b ON b.charge_id = c.id JOIN enrolment e ON e.id = c.enrolment_id
		WHERE c.entity_id = ANY($1) AND e.school_year_id = $2 AND c.kind = 'penalty'
		GROUP BY 1 ORDER BY 1`, scope(r), y.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title: fmt.Sprintf("Multas: %d", y.year),
		Columns: []report.Column{
			{Key: "month", Label: "Mês da mensalidade", Type: report.Text}, {Key: "count", Label: "Multas", Type: report.Integer},
			{Key: "charged", Label: "Aplicadas", Type: report.Money}, {Key: "paid", Label: "Pagas", Type: report.Money},
			{Key: "waived", Label: "Isentas / anuladas", Type: report.Money}, {Key: "open", Label: "Por pagar", Type: report.Money},
		},
	}
	tot := make([]money.Amount, 4)
	count := 0
	for rows.Next() {
		var m, n int
		v := make([]money.Amount, 4)
		if err := rows.Scan(&m, &n, &v[0], &v[1], &v[2], &v[3]); err != nil {
			return err
		}
		t.Rows = append(t.Rows, []any{billing.MonthNames[m], n, v[0], v[1], v[2], v[3]})
		count += n
		for i := range v {
			tot[i] += v[i]
		}
	}
	t.Totals = []any{"Total", count, tot[0], tot[1], tot[2], tot[3]}
	return report.Write(w, r, t, "multas")
}

// vat: VAT on charges by due month (non-cancelled). Discounts are listed
// separately; how they affect VAT is for the accountant to decide.
func (h *Handler) vat(w http.ResponseWriter, r *http.Request) error {
	y, err := h.year(r.Context(), r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT extract(month FROM c.due_date)::int, extract(year FROM c.due_date)::int,
		       sum(c.amount_gross)::bigint, sum(c.amount_vat)::bigint, sum(c.amount_net)::bigint,
		       COALESCE(sum(b.discount), 0)::bigint
		FROM charge c JOIN v_charge_balance b ON b.charge_id = c.id JOIN enrolment e ON e.id = c.enrolment_id
		WHERE c.entity_id = ANY($1) AND e.school_year_id = $2 AND c.cancelled_at IS NULL
		GROUP BY 2, 1 ORDER BY 2, 1`, scope(r), y.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	t := report.Table{
		Title: fmt.Sprintf("IVA sobre cobranças: %d", y.year),
		Columns: []report.Column{
			{Key: "month", Label: "Mês", Type: report.Text}, {Key: "gross", Label: "Valor bruto", Type: report.Money},
			{Key: "vat", Label: "IVA", Type: report.Money}, {Key: "net", Label: "Valor líquido", Type: report.Money},
			{Key: "discount", Label: "Descontos", Type: report.Money},
		},
	}
	tot := make([]money.Amount, 4)
	for rows.Next() {
		var m, yr int
		v := make([]money.Amount, 4)
		if err := rows.Scan(&m, &yr, &v[0], &v[1], &v[2], &v[3]); err != nil {
			return err
		}
		t.Rows = append(t.Rows, []any{fmt.Sprintf("%s %d", billing.MonthNames[m], yr), v[0], v[1], v[2], v[3]})
		for i := range v {
			tot[i] += v[i]
		}
	}
	t.Totals = []any{"Total", tot[0], tot[1], tot[2], tot[3]}
	return report.Write(w, r, t, "iva")
}
