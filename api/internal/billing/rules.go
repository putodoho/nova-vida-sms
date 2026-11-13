// Package billing holds the pure billing rules: penalty rules, due-date rules,
// charge generation and payment allocation. No database access here.
package billing

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// Penalty rule types.
const (
	PenaltyNone       = "none"
	PenaltyPercent    = "percent"
	PenaltyFixedTiers = "fixed_tiers"
)

// Tier modes for fixed_tiers.
const (
	TierReplace    = "replace"    // total penalty = amount of the highest tier reached
	TierCumulative = "cumulative" // total penalty = sum of all tiers reached
)

// PenaltyTier applies once the charge is unpaid after `AfterDay` days past the
// month's start: with due day 5, AfterDay 5 means "from the 6th".
type PenaltyTier struct {
	AfterDay int          `json:"after_day"`
	Amount   money.Amount `json:"amount"`
}

// PenaltyRule is stored as JSON on each level.
type PenaltyRule struct {
	Type      string        `json:"type"`
	RateBP    int           `json:"rate_bp,omitempty"` // percent: 500 = 5% of the remaining amount
	GraceDays int           `json:"grace_days"`        // days after the due date before a penalty applies
	Mode      string        `json:"mode,omitempty"`    // fixed_tiers
	Tiers     []PenaltyTier `json:"tiers,omitempty"`   // fixed_tiers, ordered by AfterDay
}

// ParsePenaltyRule decodes and validates a rule.
func ParsePenaltyRule(raw []byte) (PenaltyRule, error) {
	var r PenaltyRule
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("invalid penalty rule: %w", err)
	}
	return r, r.Validate()
}

// Validate checks the rule and normalises tier order.
func (r *PenaltyRule) Validate() error {
	if r.GraceDays < 0 || r.GraceDays > 60 {
		return fmt.Errorf("grace_days must be between 0 and 60")
	}
	switch r.Type {
	case PenaltyNone:
		return nil
	case PenaltyPercent:
		if r.RateBP <= 0 || r.RateBP > 10_000 {
			return fmt.Errorf("rate_bp must be between 1 and 10000")
		}
		return nil
	case PenaltyFixedTiers:
		if r.Mode != TierReplace && r.Mode != TierCumulative {
			return fmt.Errorf("mode must be %q or %q", TierReplace, TierCumulative)
		}
		if len(r.Tiers) == 0 || len(r.Tiers) > 5 {
			return fmt.Errorf("between 1 and 5 tiers are required")
		}
		sort.Slice(r.Tiers, func(i, j int) bool { return r.Tiers[i].AfterDay < r.Tiers[j].AfterDay })
		for i, t := range r.Tiers {
			if t.AfterDay < 1 || t.AfterDay > 28 {
				return fmt.Errorf("tier after_day must be between 1 and 28")
			}
			if t.Amount <= 0 {
				return fmt.Errorf("tier amount must be positive")
			}
			if i > 0 && t.AfterDay == r.Tiers[i-1].AfterDay {
				return fmt.Errorf("tiers must have distinct after_day")
			}
			if i > 0 && r.Mode == TierReplace && t.Amount <= r.Tiers[i-1].Amount {
				return fmt.Errorf("in replace mode each tier must be larger than the previous one")
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown penalty rule type %q", r.Type)
	}
}

// DueRule says when a one-off fee falls due.
type DueRule struct {
	Date        string `json:"date,omitempty"`         // fixed date, YYYY-MM-DD
	OnEnrolment bool   `json:"on_enrolment,omitempty"` // due on the enrolment confirmation date
}

// ParseDueRule decodes and validates a due rule for a fee of the given kind.
// Monthly fees use the school year's due day and must have an empty rule.
func ParseDueRule(raw []byte, kind string) (DueRule, error) {
	var d DueRule
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("invalid due rule: %w", err)
	}
	switch kind {
	case "monthly":
		if d.Date != "" || d.OnEnrolment {
			return d, fmt.Errorf("monthly fees use the school year due day")
		}
	case "one_off":
		if (d.Date == "") == !d.OnEnrolment {
			return d, fmt.Errorf("one-off fees need either a date or on_enrolment")
		}
		if d.Date != "" {
			if _, err := time.Parse("2006-01-02", d.Date); err != nil {
				return d, fmt.Errorf("date must be YYYY-MM-DD")
			}
		}
	default:
		return d, fmt.Errorf("unknown fee kind %q", kind)
	}
	return d, nil
}
