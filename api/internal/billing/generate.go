package billing

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// MonthNames are the Portuguese month names used in charge descriptions.
var MonthNames = [...]string{"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
	"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}

// Calendar is the billing calendar of a school year.
type Calendar struct {
	Year          int
	BillingMonths []int // e.g. 2..11
	DueDay        int
}

// DueDate is the due date of a monthly charge for month m.
func (c Calendar) DueDate(m int) time.Time {
	return time.Date(c.Year, time.Month(m), c.DueDay, 0, 0, 0, 0, time.UTC)
}

// Fee is a catalogue item as needed for generation.
type Fee struct {
	ID        uuid.UUID
	Name      string
	Kind      string // monthly | one_off
	Amount    money.Amount
	VATMode   string
	VATRateBP int
	Due       DueRule
}

// Draft is a charge to be inserted.
type Draft struct {
	FeeItemID   uuid.UUID
	Kind        string
	PeriodMonth int // 0 for one-off
	Description string
	DueDate     time.Time
	Lines       money.Lines
	VATRateBP   int
}

// Generate builds the charges of an enrolment.
//
// Monthly fees produce one charge per billing month from fromMonth onwards.
// One-off fees are due on their fixed date, or on the enrolment date when the
// rule says so or when the fixed date is already past.
func Generate(cal Calendar, fromMonth int, fees []Fee, vatMethod string, enrolledOn time.Time) ([]Draft, error) {
	enrolledOn = dateOnly(enrolledOn)
	var out []Draft
	for _, f := range fees {
		lines, err := money.SplitVAT(f.Amount, f.VATMode, f.VATRateBP, vatMethod)
		if err != nil {
			return nil, fmt.Errorf("fee %s: %w", f.Name, err)
		}
		rate := f.VATRateBP
		if f.VATMode == money.ModeNone {
			rate = 0
		}
		switch f.Kind {
		case "monthly":
			for _, m := range cal.BillingMonths {
				if m < fromMonth {
					continue
				}
				out = append(out, Draft{
					FeeItemID: f.ID, Kind: "monthly", PeriodMonth: m,
					Description: fmt.Sprintf("%s – %s %d", f.Name, MonthNames[m], cal.Year),
					DueDate:     cal.DueDate(m), Lines: lines, VATRateBP: rate,
				})
			}
		case "one_off":
			due := enrolledOn
			if f.Due.Date != "" {
				d, err := time.Parse(time.DateOnly, f.Due.Date)
				if err != nil {
					return nil, fmt.Errorf("fee %s: %w", f.Name, err)
				}
				if d.After(enrolledOn) {
					due = d
				}
			}
			out = append(out, Draft{
				FeeItemID: f.ID, Kind: "one_off", Description: f.Name, DueDate: due, Lines: lines, VATRateBP: rate,
			})
		default:
			return nil, fmt.Errorf("fee %s: unknown kind %q", f.Name, f.Kind)
		}
	}
	return out, nil
}

// Charge statuses (derived, never stored).
const (
	StatusPaid      = "paid"
	StatusPartial   = "partial"
	StatusOverdue   = "overdue"
	StatusUpcoming  = "upcoming"
	StatusCancelled = "cancelled"
)

// Status derives a charge's status. A partly paid charge past its due date is
// overdue: what matters to the school is that money is late.
func Status(cancelled bool, payable, paid money.Amount, due, today time.Time) string {
	switch {
	case cancelled:
		return StatusCancelled
	case paid >= payable:
		return StatusPaid
	case dateOnly(due).Before(dateOnly(today)):
		return StatusOverdue
	case paid > 0:
		return StatusPartial
	default:
		return StatusUpcoming
	}
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
