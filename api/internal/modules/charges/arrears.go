package charges

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
	"github.com/putodoho/nova-vida-sms/api/internal/report"
)

// PupilArrears is one pupil's overdue debt: the calling list of the
// "Cobranças e dívidas" screen.
type PupilArrears struct {
	PupilID     uuid.UUID    `json:"pupil_id"`
	EntityID    uuid.UUID    `json:"entity_id"`
	Number      string       `json:"number"`
	FullName    string       `json:"full_name"`
	ClassName   *string      `json:"class_name"`
	PayerName   *string      `json:"payer_name"`
	PayerPhone  *string      `json:"payer_phone"`
	PayerPhone2 *string      `json:"payer_phone2"`
	Overdue     money.Amount `json:"overdue"`     // due and unpaid
	Outstanding money.Amount `json:"outstanding"` // everything still to pay this year
	Charges     int          `json:"charges"`     // overdue charges
	OldestDue   string       `json:"oldest_due"`
	DaysOverdue int          `json:"days_overdue"` // since the oldest unpaid due date
}

// arrears groups overdue charges per pupil and entity. Filters: year,
// class_id, min_days (oldest debt at least N days), q. ?format=xlsx returns
// the list as a spreadsheet (for phoning guardians).
func (h *Handler) arrears(w http.ResponseWriter, r *http.Request) error {
	qs := r.URL.Query()
	today := h.today()
	var classID *uuid.UUID
	if v := qs.Get("class_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return httpx.BadRequest("class_id inválido.")
		}
		classID = &id
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.id, b.entity_id, p.number, p.full_name, cl.name, g.full_name, g.phone1, NULLIF(g.phone2, ''),
		       COALESCE(sum(b.remaining) FILTER (WHERE b.due_date < $3), 0)::bigint,
		       sum(b.remaining)::bigint,
		       count(*) FILTER (WHERE b.due_date < $3)::int,
		       min(b.due_date) FILTER (WHERE b.due_date < $3)
		FROM v_charge_balance b
		JOIN enrolment e ON e.id = b.enrolment_id
		JOIN school_year sy ON sy.id = e.school_year_id
		JOIN pupil p ON p.id = b.pupil_id
		LEFT JOIN class cl ON cl.id = e.class_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE b.entity_id = ANY($1)
		  AND (($2 = 0 AND sy.id = (SELECT id FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1)) OR sy.year = $2)
		  AND NOT b.cancelled AND b.remaining > 0
		  AND ($4::uuid IS NULL OR e.class_id = $4)
		  AND ($5 = '' OR p.full_name ILIKE '%' || $5 || '%' OR p.number ILIKE $5 || '%' OR g.full_name ILIKE '%' || $5 || '%')
		GROUP BY p.id, b.entity_id, cl.name, g.full_name, g.phone1, g.phone2
		HAVING min(b.due_date) FILTER (WHERE b.due_date < $3) <= $3::date - GREATEST($6, 1)
		ORDER BY 9 DESC, p.full_name`,
		auth.EntityScope(r.Context()).EntityIDs, httpx.QueryInt(r, "year", 0), today, classID,
		strings.TrimSpace(qs.Get("q")), httpx.QueryInt(r, "min_days", 0))
	if err != nil {
		return err
	}
	items := []PupilArrears{}
	for rows.Next() {
		var a PupilArrears
		var oldest time.Time
		if err := rows.Scan(&a.PupilID, &a.EntityID, &a.Number, &a.FullName, &a.ClassName, &a.PayerName, &a.PayerPhone, &a.PayerPhone2,
			&a.Overdue, &a.Outstanding, &a.Charges, &oldest); err != nil {
			rows.Close()
			return err
		}
		a.OldestDue = oldest.Format(time.DateOnly)
		a.DaysOverdue = int(today.Sub(oldest).Hours() / 24)
		items = append(items, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if qs.Get("format") != "xlsx" {
		httpx.JSON(w, http.StatusOK, httpx.List[PupilArrears]{Items: items, Total: len(items)})
		return nil
	}
	t := report.Table{
		Title: fmt.Sprintf("Dívidas por aluno a %s", today.Format("02/01/2006")),
		Columns: []report.Column{
			{Key: "number", Label: "Nº", Type: report.Text}, {Key: "pupil", Label: "Aluno", Type: report.Text},
			{Key: "class", Label: "Turma", Type: report.Text}, {Key: "payer", Label: "Encarregado", Type: report.Text},
			{Key: "phone", Label: "Telefone", Type: report.Text}, {Key: "phone2", Label: "Telefone 2", Type: report.Text},
			{Key: "overdue", Label: "Em dívida", Type: report.Money}, {Key: "outstanding", Label: "Total por pagar", Type: report.Money},
			{Key: "oldest", Label: "Mais antiga", Type: report.Date}, {Key: "days", Label: "Dias de atraso", Type: report.Integer},
		},
	}
	var to, ts money.Amount
	for _, a := range items {
		t.Rows = append(t.Rows, []any{a.Number, a.FullName, a.ClassName, a.PayerName, a.PayerPhone, a.PayerPhone2, a.Overdue, a.Outstanding, a.OldestDue, a.DaysOverdue})
		to += a.Overdue
		ts += a.Outstanding
	}
	pupils := fmt.Sprintf("%d alunos", len(items))
	if len(items) == 1 {
		pupils = "1 aluno"
	}
	t.Totals = []any{"Total", pupils, nil, nil, nil, nil, to, ts, nil, nil}
	return report.Write(w, r, t, "dividas-por-aluno")
}
