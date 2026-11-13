package enrolments

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// AccountCharge is one line of the pupil's account (conta corrente).
type AccountCharge struct {
	ID          uuid.UUID    `json:"id"`
	EntityID    uuid.UUID    `json:"entity_id"`
	Kind        string       `json:"kind"`
	PeriodMonth *int         `json:"period_month"`
	Description string       `json:"description"`
	DueDate     string       `json:"due_date"`
	Gross       money.Amount `json:"gross"`
	VAT         money.Amount `json:"vat"`
	Discount    money.Amount `json:"discount"`
	Payable     money.Amount `json:"payable"`
	Paid        money.Amount `json:"paid"`
	Remaining   money.Amount `json:"remaining"`
	Status      string       `json:"status"`
	ParentID    *uuid.UUID   `json:"parent_charge_id"`
	CancelNote  string       `json:"cancel_reason,omitempty"`
}

// AccountSummary: Em dívida = due and unpaid; Total por pagar = everything
// still open this year; Crédito = payments not yet allocated.
type AccountSummary struct {
	Overdue     money.Amount `json:"overdue"`
	Outstanding money.Amount `json:"outstanding"`
	Paid        money.Amount `json:"paid"`
	Credit      money.Amount `json:"credit"`
	Total       money.Amount `json:"total"` // payable of all non-cancelled charges
}

type Account struct {
	PupilID uuid.UUID       `json:"pupil_id"`
	Year    int             `json:"year"`
	Summary AccountSummary  `json:"summary"`
	Charges []AccountCharge `json:"charges"`
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request) error {
	pupilID, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	scope := auth.EntityScope(r.Context())
	year := httpx.QueryInt(r, "year", 0)
	if year == 0 {
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT year FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1`).Scan(&year); err != nil {
			return err
		}
	}
	today := h.today()
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id, c.entity_id, c.kind, c.period_month, c.description, c.due_date, c.amount_gross, c.amount_vat,
		       b.discount, b.payable, b.paid, b.remaining, b.cancelled, c.parent_charge_id, c.cancel_reason
		FROM charge c
		JOIN v_charge_balance b ON b.charge_id = c.id
		JOIN enrolment e ON e.id = c.enrolment_id
		JOIN school_year sy ON sy.id = e.school_year_id
		WHERE c.pupil_id = $1 AND c.entity_id = ANY($2) AND sy.year = $3
		ORDER BY c.due_date, (c.kind = 'penalty') DESC, c.kind, c.description`, pupilID, scope.EntityIDs, year)
	if err != nil {
		return err
	}
	acc := Account{PupilID: pupilID, Year: year, Charges: []AccountCharge{}}
	for rows.Next() {
		var c AccountCharge
		var due time.Time
		var cancelled bool
		if err := rows.Scan(&c.ID, &c.EntityID, &c.Kind, &c.PeriodMonth, &c.Description, &due, &c.Gross, &c.VAT,
			&c.Discount, &c.Payable, &c.Paid, &c.Remaining, &cancelled, &c.ParentID, &c.CancelNote); err != nil {
			rows.Close()
			return err
		}
		c.DueDate = due.Format(time.DateOnly)
		c.Status = billing.Status(cancelled, c.Payable, c.Paid, due, today)
		if !cancelled {
			acc.Summary.Total += c.Payable
			acc.Summary.Paid += c.Paid
			acc.Summary.Outstanding += c.Remaining
			if c.Status == billing.StatusOverdue {
				acc.Summary.Overdue += c.Remaining
			}
		}
		acc.Charges = append(acc.Charges, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(sum(p.amount - COALESCE(a.total, 0)), 0)
		FROM payment p
		LEFT JOIN (SELECT payment_id, sum(amount) AS total FROM allocation WHERE active GROUP BY payment_id) a ON a.payment_id = p.id
		WHERE p.pupil_id = $1 AND p.entity_id = ANY($2) AND p.cancelled_at IS NULL`, pupilID, scope.EntityIDs).
		Scan(&acc.Summary.Credit); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, acc)
	return nil
}

// ListItem is one row of the pupils list: an enrolment with its pupil, main
// guardian and (with charges.read) balance.
type ListItem struct {
	EnrolmentID uuid.UUID     `json:"enrolment_id"`
	EntityID    uuid.UUID     `json:"entity_id"`
	PupilID     uuid.UUID     `json:"pupil_id"`
	Number      string        `json:"number"`
	FullName    string        `json:"full_name"`
	BirthDate   string        `json:"birth_date"`
	ClassID     *uuid.UUID    `json:"class_id"`
	ClassName   *string       `json:"class_name"`
	Status      string        `json:"status"`
	Type        string        `json:"type"`
	PayerName   *string       `json:"payer_name"`
	PayerPhone  *string       `json:"payer_phone"`
	HealthAlert bool          `json:"health_alert"`
	Overdue     *money.Amount `json:"overdue,omitempty"`
	Outstanding *money.Amount `json:"outstanding,omitempty"`
}

// list returns the year's enrolments in scope. Filters: year, class_id,
// status, q (name, number, guardian phone), debt=overdue.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	scope := auth.EntityScope(r.Context())
	qs := r.URL.Query()
	page := httpx.ParsePage(r)
	year := httpx.QueryInt(r, "year", 0)
	var classID *uuid.UUID
	if v := qs.Get("class_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return httpx.BadRequest("class_id inválido.")
		}
		classID = &id
	}
	status := qs.Get("status")
	q := strings.TrimSpace(qs.Get("q"))
	onlyOverdue := qs.Get("debt") == "overdue"
	canMoney := auth.CurrentUser(r.Context()).Can(auth.ChargesRead)

	items := []ListItem{}
	total := 0
	rows, err := h.Pool.Query(r.Context(), `
		WITH yr AS (
			SELECT id FROM school_year
			WHERE ($1 = 0 AND id = (SELECT id FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1)) OR year = $1
		), bal AS (
			SELECT b.enrolment_id,
			       sum(b.remaining) FILTER (WHERE b.due_date < $7) AS overdue,
			       sum(b.remaining) AS outstanding
			FROM v_charge_balance b WHERE NOT b.cancelled GROUP BY b.enrolment_id
		)
		SELECT e.id, e.entity_id, p.id, p.number, p.full_name, p.birth_date::text, e.class_id, c.name, e.status, e.type,
		       g.full_name, g.phone1,
		       COALESCE(hr.chronic_condition <> '' OR hr.allergies <> '' OR hr.diet_restrictions <> '' OR hr.medication <> '', false),
		       COALESCE(bal.overdue, 0)::bigint, COALESCE(bal.outstanding, 0)::bigint,
		       count(*) OVER ()::int
		FROM enrolment e
		JOIN pupil p ON p.id = e.pupil_id
		LEFT JOIN class c ON c.id = e.class_id
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		LEFT JOIN health_record hr ON hr.pupil_id = p.id
		LEFT JOIN bal ON bal.enrolment_id = e.id
		WHERE e.entity_id = ANY($2) AND e.school_year_id IN (SELECT id FROM yr)
		  AND ($3::uuid IS NULL OR e.class_id = $3)
		  AND ($4 = '' OR e.status = $4)
		  AND ($5 = '' OR p.full_name ILIKE '%' || $5 || '%' OR p.number ILIKE $5 || '%' OR g.phone1 LIKE '%' || $5 || '%')
		  AND (NOT $6 OR COALESCE(bal.overdue, 0) > 0)
		ORDER BY c.name NULLS LAST, p.full_name
		LIMIT $8 OFFSET $9`,
		year, scope.EntityIDs, classID, status, q, onlyOverdue && canMoney, h.today(), page.PageSize, page.Offset())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it ListItem
		var overdue, outstanding money.Amount
		if err := rows.Scan(&it.EnrolmentID, &it.EntityID, &it.PupilID, &it.Number, &it.FullName, &it.BirthDate, &it.ClassID,
			&it.ClassName, &it.Status, &it.Type, &it.PayerName, &it.PayerPhone, &it.HealthAlert, &overdue, &outstanding, &total); err != nil {
			return err
		}
		if canMoney {
			it.Overdue, it.Outstanding = &overdue, &outstanding
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[ListItem]{Items: items, Total: total})
	return nil
}
