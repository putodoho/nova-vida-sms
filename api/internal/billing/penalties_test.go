package billing

import (
	"testing"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

func TestPercentPenalty(t *testing.T) {
	rule := PenaltyRule{Type: PenaltyPercent, RateBP: 500}
	c := OverdueCharge{ID: uuid.New(), Description: "Mensalidade – Março 2027", DueDate: d("2027-03-05"), Remaining: money.MT(5300)}

	if p := Penalties(rule, c, d("2027-03-05")); len(p) != 0 {
		t.Fatalf("on due date: %+v", p)
	}
	p := Penalties(rule, c, d("2027-03-06"))
	if len(p) != 1 || p[0].Amount != money.MT(265) || p[0].Tier != 1 || p[0].DueDate != c.DueDate || p[0].Description != "Multa – Mensalidade – Março 2027" {
		t.Fatalf("day after: %+v", p)
	}
	// Partly paid: 5% of what is still owed.
	c.Remaining = money.MT(2300)
	if p := Penalties(rule, c, d("2027-03-20")); p[0].Amount != money.MT(115) {
		t.Fatalf("partial: %+v", p)
	}
	c.ExistingTiers = map[int]bool{1: true}
	if p := Penalties(rule, c, d("2027-04-20")); len(p) != 0 {
		t.Fatalf("charged twice: %+v", p)
	}
	rule.GraceDays = 3
	c.ExistingTiers = nil
	if p := Penalties(rule, c, d("2027-03-08")); len(p) != 0 {
		t.Fatalf("within grace: %+v", p)
	}
	c.Remaining = 0
	if p := Penalties(rule, c, d("2027-06-01")); len(p) != 0 {
		t.Fatalf("paid charge: %+v", p)
	}
}

// Preschool: 100 MT from the 6th, 200 MT after the 10th.
func TestTierPenalty(t *testing.T) {
	tiers := []PenaltyTier{{AfterDay: 5, Amount: money.MT(100)}, {AfterDay: 10, Amount: money.MT(200)}}
	c := OverdueCharge{ID: uuid.New(), Description: "Mensalidade Sala 2 – Março 2027", DueDate: d("2027-03-05"), Remaining: money.MT(3000)}
	sum := func(ps []PenaltyDraft) money.Amount {
		var s money.Amount
		for _, p := range ps {
			s += p.Amount
		}
		return s
	}

	replace := PenaltyRule{Type: PenaltyFixedTiers, Mode: TierReplace, Tiers: tiers}
	if p := Penalties(replace, c, d("2027-03-05")); len(p) != 0 {
		t.Fatalf("on the 5th: %+v", p)
	}
	if p := Penalties(replace, c, d("2027-03-06")); len(p) != 1 || p[0].Amount != money.MT(100) {
		t.Fatalf("on the 6th: %+v", p)
	}
	if p := Penalties(replace, c, d("2027-03-10")); sum(p) != money.MT(100) {
		t.Fatalf("on the 10th: %+v", p)
	}
	// Job missed a few days: both tiers at once, total 200.
	if p := Penalties(replace, c, d("2027-03-11")); len(p) != 2 || sum(p) != money.MT(200) {
		t.Fatalf("on the 11th (replace): %+v", p)
	}
	// Tier 1 already charged: only the 100 MT difference.
	c.ExistingTiers = map[int]bool{1: true}
	if p := Penalties(replace, c, d("2027-03-11")); len(p) != 1 || p[0].Tier != 2 || p[0].Amount != money.MT(100) {
		t.Fatalf("tier 2 only: %+v", p)
	}

	cumulative := PenaltyRule{Type: PenaltyFixedTiers, Mode: TierCumulative, Tiers: tiers}
	c.ExistingTiers = nil
	if p := Penalties(cumulative, c, d("2027-03-11")); sum(p) != money.MT(300) {
		t.Fatalf("cumulative: %+v", p)
	}
	if p := Penalties(PenaltyRule{Type: PenaltyNone}, c, d("2027-06-01")); len(p) != 0 {
		t.Fatalf("none: %+v", p)
	}
}

func TestDiscountAmount(t *testing.T) {
	cases := []struct {
		mode             string
		value            int64
		gross, remaining money.Amount
		want             money.Amount
	}{
		{"percent", 1000, money.MT(2800), money.MT(2800), money.MT(280)},
		{"percent", 10000, money.MT(2800), money.MT(1000), money.MT(1000)}, // capped at unpaid
		{"fixed", 50000, money.MT(2800), money.MT(2800), money.MT(500)},
		{"fixed", 50000, money.MT(2800), 0, 0},
		{"bogus", 50000, money.MT(2800), money.MT(2800), 0},
	}
	for _, c := range cases {
		if got := DiscountAmount(c.mode, c.value, c.gross, c.remaining); got != c.want {
			t.Errorf("%+v: got %s", c, got.Format())
		}
	}
}
