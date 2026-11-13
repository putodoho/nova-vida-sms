package billing

import (
	"testing"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

func TestAllocate(t *testing.T) {
	feb, mar, apr := uuid.New(), uuid.New(), uuid.New()
	penFeb, books := uuid.New(), uuid.New()
	charges := func() []OpenCharge {
		return []OpenCharge{
			{ID: apr, Kind: "monthly", DueDate: d("2027-04-05"), Remaining: money.MT(5300)},
			{ID: books, Kind: "one_off", DueDate: d("2027-02-05"), Remaining: money.MT(750)},
			{ID: mar, Kind: "monthly", DueDate: d("2027-03-05"), Remaining: money.MT(5300)},
			{ID: feb, Kind: "monthly", DueDate: d("2027-02-05"), Remaining: money.MT(2300)}, // partly paid
			{ID: penFeb, Kind: "penalty", DueDate: d("2027-02-05"), Remaining: money.MT(265)},
		}
	}

	t.Run("oldest first, penalty before fee, one-off last on same day", func(t *testing.T) {
		allocs, credit := Allocate(money.MT(3000), charges())
		want := []Allocation{{penFeb, money.MT(265)}, {feb, money.MT(2300)}, {books, money.MT(435)}}
		if credit != 0 || len(allocs) != len(want) {
			t.Fatalf("got %+v credit %d", allocs, credit)
		}
		for i := range want {
			if allocs[i] != want[i] {
				t.Errorf("alloc %d: got %+v want %+v", i, allocs[i], want[i])
			}
		}
	})

	t.Run("advance payment covers future months, rest is credit", func(t *testing.T) {
		total := money.MT(265 + 2300 + 750 + 5300 + 5300)
		allocs, credit := Allocate(total+money.MT(100), charges())
		if len(allocs) != 5 || credit != money.MT(100) {
			t.Fatalf("got %d allocations, credit %s", len(allocs), credit.Format())
		}
		var sum money.Amount
		for _, a := range allocs {
			sum += a.Amount
		}
		if sum != total {
			t.Fatalf("allocated %s", sum.Format())
		}
	})

	t.Run("nothing open", func(t *testing.T) {
		allocs, credit := Allocate(money.MT(500), nil)
		if len(allocs) != 0 || credit != money.MT(500) {
			t.Fatalf("got %+v %d", allocs, credit)
		}
	})
}
