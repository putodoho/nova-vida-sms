package billing

import (
	"time"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// OverdueCharge is a monthly charge that may attract a penalty.
type OverdueCharge struct {
	ID          uuid.UUID
	Description string
	DueDate     time.Time
	Remaining   money.Amount
	// ExistingTiers are tiers already charged (including waived ones), so a
	// penalty is never charged twice and a waiver is never undone.
	ExistingTiers map[int]bool
}

// PenaltyDraft is a penalty charge to insert.
type PenaltyDraft struct {
	ParentID    uuid.UUID
	Tier        int
	Amount      money.Amount
	DueDate     time.Time // same as the parent: allocation settles it just before the fee
	Description string
}

// Penalties returns the penalties due today for a charge under rule.
//
// percent: one penalty (tier 1) of rate × remaining once today is past
// due + grace days.
// fixed_tiers: tier i applies once today is past day AfterDay (+ grace) of
// the due month. In replace mode each tier adds the difference to the
// previous tier, so the total equals the highest tier reached; in cumulative
// mode every tier adds its full amount.
func Penalties(rule PenaltyRule, c OverdueCharge, today time.Time) []PenaltyDraft {
	if c.Remaining <= 0 {
		return nil
	}
	today = dateOnly(today)
	due := dateOnly(c.DueDate)
	desc := "Multa – " + c.Description
	var out []PenaltyDraft
	switch rule.Type {
	case PenaltyPercent:
		if today.After(due.AddDate(0, 0, rule.GraceDays)) && !c.ExistingTiers[1] {
			if amt := c.Remaining.MulBP(rule.RateBP); amt > 0 {
				out = append(out, PenaltyDraft{c.ID, 1, amt, due, desc})
			}
		}
	case PenaltyFixedTiers:
		var prev money.Amount
		for i, t := range rule.Tiers {
			tier := i + 1
			threshold := time.Date(due.Year(), due.Month(), t.AfterDay, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rule.GraceDays)
			amt := t.Amount
			if rule.Mode == TierReplace {
				amt = t.Amount - prev
			}
			prev = t.Amount
			if !today.After(threshold) || c.ExistingTiers[tier] || amt <= 0 {
				continue
			}
			out = append(out, PenaltyDraft{c.ID, tier, amt, due, desc})
		}
	}
	return out
}

// DiscountAmount computes the discount on one charge: percent of the gross
// amount (value in basis points) or a fixed amount (centavos), never more
// than what is still unpaid.
func DiscountAmount(mode string, value int64, gross, remaining money.Amount) money.Amount {
	var amt money.Amount
	switch mode {
	case "percent":
		amt = gross.MulBP(int(value))
	case "fixed":
		amt = money.Amount(value)
	}
	return max(0, min(amt, remaining))
}
