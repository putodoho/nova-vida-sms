package billing

import "testing"

func TestParsePenaltyRule(t *testing.T) {
	valid := []string{
		`{"type":"none"}`,
		`{"type":"percent","rate_bp":500,"grace_days":0}`,
		`{"type":"fixed_tiers","mode":"replace","tiers":[{"after_day":10,"amount":20000},{"after_day":5,"amount":10000}]}`,
		`{"type":"fixed_tiers","mode":"cumulative","tiers":[{"after_day":5,"amount":10000},{"after_day":10,"amount":10000}]}`,
	}
	for _, s := range valid {
		r, err := ParsePenaltyRule([]byte(s))
		if err != nil {
			t.Errorf("%s: %v", s, err)
		}
		if r.Type == PenaltyFixedTiers && r.Tiers[0].AfterDay != 5 {
			t.Errorf("tiers not sorted: %+v", r.Tiers)
		}
	}
	invalid := []string{
		`{"type":"bogus"}`,
		`{"type":"percent","rate_bp":0}`,
		`{"type":"percent","rate_bp":500,"grace_days":-1}`,
		`{"type":"fixed_tiers","mode":"replace","tiers":[]}`,
		`{"type":"fixed_tiers","mode":"x","tiers":[{"after_day":5,"amount":100}]}`,
		`{"type":"fixed_tiers","mode":"replace","tiers":[{"after_day":5,"amount":200},{"after_day":10,"amount":100}]}`,
		`{"type":"fixed_tiers","mode":"replace","tiers":[{"after_day":5,"amount":100},{"after_day":5,"amount":200}]}`,
		`{"type":"fixed_tiers","mode":"replace","tiers":[{"after_day":40,"amount":100}]}`,
		`not json`,
	}
	for _, s := range invalid {
		if _, err := ParsePenaltyRule([]byte(s)); err == nil {
			t.Errorf("%s: expected error", s)
		}
	}
}

func TestParseDueRule(t *testing.T) {
	cases := []struct {
		raw, kind string
		ok        bool
	}{
		{`{}`, "monthly", true},
		{``, "monthly", true},
		{`{"date":"2027-01-30"}`, "monthly", false},
		{`{"date":"2027-01-30"}`, "one_off", true},
		{`{"on_enrolment":true}`, "one_off", true},
		{`{}`, "one_off", false},
		{`{"date":"2027-01-30","on_enrolment":true}`, "one_off", false},
		{`{"date":"30/01/2027"}`, "one_off", false},
		{`{}`, "weekly", false},
	}
	for _, c := range cases {
		_, err := ParseDueRule([]byte(c.raw), c.kind)
		if (err == nil) != c.ok {
			t.Errorf("%s %s: err=%v want ok=%v", c.raw, c.kind, err, c.ok)
		}
	}
}
