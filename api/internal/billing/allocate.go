package billing

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// OpenCharge is a charge with money still to pay.
type OpenCharge struct {
	ID        uuid.UUID
	Kind      string // monthly | one_off | penalty
	DueDate   time.Time
	Remaining money.Amount
}

// Allocation assigns part of a payment to a charge.
type Allocation struct {
	ChargeID uuid.UUID
	Amount   money.Amount
}

// kindOrder: on the same due date, penalties are settled first, then monthly
// fees, then one-off items.
var kindOrder = map[string]int{"penalty": 0, "monthly": 1, "one_off": 2}

// SortForAllocation orders charges oldest first (see kindOrder for ties).
func SortForAllocation(charges []OpenCharge) {
	sort.SliceStable(charges, func(i, j int) bool {
		a, b := charges[i], charges[j]
		if !a.DueDate.Equal(b.DueDate) {
			return a.DueDate.Before(b.DueDate)
		}
		return kindOrder[a.Kind] < kindOrder[b.Kind]
	})
}

// Allocate spreads amount over the charges, oldest first, including charges
// not yet due (advance payment). It returns the allocations and what is left
// over as credit. The input slice is reordered.
func Allocate(amount money.Amount, charges []OpenCharge) ([]Allocation, money.Amount) {
	SortForAllocation(charges)
	var out []Allocation
	left := amount
	for _, c := range charges {
		if left <= 0 {
			break
		}
		if c.Remaining <= 0 {
			continue
		}
		take := min(c.Remaining, left)
		out = append(out, Allocation{ChargeID: c.ID, Amount: take})
		left -= take
	}
	return out, left
}
