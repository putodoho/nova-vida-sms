// Package money handles amounts in Mozambican Metical stored as integer
// centavos, VAT calculation and display formatting. Never use floats for money.
package money

import (
	"fmt"
	"strings"
)

// Amount is a value in centavos (1 MT = 100).
type Amount int64

// MT builds an Amount from whole meticais.
func MT(meticais int64) Amount { return Amount(meticais * 100) }

// MulBP multiplies by a rate in basis points (500 = 5%), rounding half up.
func (a Amount) MulBP(bp int) Amount {
	return Amount(divRoundHalfUp(int64(a)*int64(bp), 10_000))
}

// Format renders the amount as "5.035,00 MT".
func (a Amount) Format() string {
	v := int64(a)
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	units, cents := v/100, v%100
	s := fmt.Sprintf("%d", units)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s%s,%02d MT", sign, b.String(), cents)
}

// VAT methods, configured per entity.
const (
	VATAdded            = "added"             // price is net; VAT is added on top
	VATIncludedStandard = "included_standard" // price is gross; VAT = gross × r / (1 + r)
	VATIncludedFlat     = "included_flat"     // price is gross; VAT = gross × r (school's current template)
)

// Fee item VAT modes.
const (
	ModeNone     = "none"
	ModeIncluded = "included"
	ModeAdded    = "added"
)

// Lines is a priced line split into net, VAT and gross.
type Lines struct {
	Net   Amount `json:"net"`
	VAT   Amount `json:"vat"`
	Gross Amount `json:"gross"`
}

// SplitVAT computes the net/VAT/gross of a catalogue price.
//
// mode is the fee item's VAT mode ("none", "included", "added"); method is the
// entity's VAT method, used to decide how included VAT is extracted.
func SplitVAT(price Amount, mode string, rateBP int, method string) (Lines, error) {
	if price < 0 {
		return Lines{}, fmt.Errorf("negative price")
	}
	switch mode {
	case ModeNone, "":
		return Lines{Net: price, Gross: price}, nil
	case ModeAdded:
		vat := price.MulBP(rateBP)
		return Lines{Net: price, VAT: vat, Gross: price + vat}, nil
	case ModeIncluded:
		var vat Amount
		switch method {
		case VATIncludedFlat:
			vat = price.MulBP(rateBP)
		case VATIncludedStandard, VATAdded, "":
			// gross × r / (1 + r) with r in basis points.
			vat = Amount(divRoundHalfUp(int64(price)*int64(rateBP), 10_000+int64(rateBP)))
		default:
			return Lines{}, fmt.Errorf("unknown VAT method %q", method)
		}
		return Lines{Net: price - vat, VAT: vat, Gross: price}, nil
	default:
		return Lines{}, fmt.Errorf("unknown VAT mode %q", mode)
	}
}

// divRoundHalfUp divides two non-negative-denominator integers, rounding half
// away from zero.
func divRoundHalfUp(n, d int64) int64 {
	if n >= 0 {
		return (n + d/2) / d
	}
	return -((-n + d/2) / d)
}
