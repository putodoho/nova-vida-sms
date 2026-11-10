package money

import "testing"

func TestFormat(t *testing.T) {
	cases := map[Amount]string{
		0:              "0,00 MT",
		MT(5035):       "5.035,00 MT",
		MT(50350):      "50.350,00 MT",
		Amount(13333):  "133,33 MT",
		MT(1_234_567):  "1.234.567,00 MT",
		Amount(-26500): "-265,00 MT",
		Amount(99):     "0,99 MT",
	}
	for in, want := range cases {
		if got := in.Format(); got != want {
			t.Errorf("Format(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitVAT(t *testing.T) {
	cases := []struct {
		name   string
		price  Amount
		mode   string
		method string
		want   Lines
	}{
		{"none", MT(2800), ModeNone, VATIncludedFlat, Lines{MT(2800), 0, MT(2800)}},
		// School's current statement: 2.800 → VAT 140 → net 2.660.
		{"included flat", MT(2800), ModeIncluded, VATIncludedFlat, Lines{MT(2660), MT(140), MT(2800)}},
		// Standard extraction: 2.800 / 1,05 → VAT 133,33.
		{"included standard", MT(2800), ModeIncluded, VATIncludedStandard, Lines{Amount(266667), Amount(13333), MT(2800)}},
		{"added", MT(2800), ModeAdded, VATIncludedFlat, Lines{MT(2800), MT(140), MT(2940)}},
	}
	for _, c := range cases {
		got, err := SplitVAT(c.price, c.mode, 500, c.method)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
		if got.Net+got.VAT != got.Gross {
			t.Errorf("%s: net + vat != gross", c.name)
		}
	}
}

// The client's 2027 statement: 3 services, flat 5% included VAT, 10 months.
func TestSchoolStatementExample(t *testing.T) {
	var monthlyNet, monthlyVAT Amount
	for _, p := range []Amount{MT(2800), MT(1000), MT(1500)} {
		l, err := SplitVAT(p, ModeIncluded, 500, VATIncludedFlat)
		if err != nil {
			t.Fatal(err)
		}
		monthlyNet += l.Net
		monthlyVAT += l.VAT
	}
	if monthlyNet != MT(5035) || monthlyVAT != MT(265) {
		t.Fatalf("monthly net %s vat %s", monthlyNet.Format(), monthlyVAT.Format())
	}
	if annual := monthlyNet * 10; annual != MT(50350) {
		t.Fatalf("annual net %s, want 50.350,00 MT", annual.Format())
	}
}

func TestSplitVATErrors(t *testing.T) {
	if _, err := SplitVAT(MT(1), "bogus", 500, VATIncludedFlat); err == nil {
		t.Error("expected error for unknown mode")
	}
	if _, err := SplitVAT(MT(-1), ModeNone, 0, ""); err == nil {
		t.Error("expected error for negative price")
	}
}
