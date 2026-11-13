package money

import "strings"

var (
	units = []string{"zero", "um", "dois", "três", "quatro", "cinco", "seis", "sete", "oito", "nove",
		"dez", "onze", "doze", "treze", "catorze", "quinze", "dezasseis", "dezassete", "dezoito", "dezanove"}
	tens     = []string{"", "", "vinte", "trinta", "quarenta", "cinquenta", "sessenta", "setenta", "oitenta", "noventa"}
	hundreds = []string{"", "cento", "duzentos", "trezentos", "quatrocentos", "quinhentos", "seiscentos",
		"setecentos", "oitocentos", "novecentos"}
)

// below1000 spells 1..999.
func below1000(n int64) string {
	if n == 100 {
		return "cem"
	}
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, hundreds[h])
	}
	if r := n % 100; r > 0 {
		if r < 20 {
			parts = append(parts, units[r])
		} else {
			t := tens[r/10]
			if u := r % 10; u > 0 {
				t += " e " + units[u]
			}
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " e ")
}

// NumberWords spells a non-negative integer in Portuguese (Mozambique /
// Portugal usage): 5035 → "cinco mil e trinta e cinco".
func NumberWords(n int64) string {
	if n == 0 {
		return "zero"
	}
	type group struct {
		value int64
		words string
	}
	scales := []struct {
		size           int64
		singular, plur string
	}{
		{1_000_000_000, "mil milhões", "mil milhões"},
		{1_000_000, "milhão", "milhões"},
		{1_000, "mil", "mil"},
		{1, "", ""},
	}
	var groups []group
	rest := n
	for _, s := range scales {
		g := rest / s.size
		rest %= s.size
		if g == 0 {
			continue
		}
		var w string
		switch {
		case s.size == 1:
			w = below1000(g)
		case s.size == 1_000 && g == 1:
			w = "mil"
		case g == 1:
			w = "um " + s.singular
		default:
			w = below1000(g) + " " + s.plur
		}
		groups = append(groups, group{g * s.size, w})
	}
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			// "e" before the last group when it is below 100 or a round hundred.
			last := g.value
			if i == len(groups)-1 && (last < 100 || (last < 1000 && last%100 == 0)) {
				b.WriteString(" e ")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(g.words)
	}
	return b.String()
}

// Words spells an amount for receipts: 503500 → "cinco mil e trinta e cinco
// meticais"; 13333 → "cento e trinta e três meticais e trinta e três centavos".
func (a Amount) Words() string {
	v := int64(a)
	if v < 0 {
		return "menos " + Amount(-v).Words()
	}
	mt, ct := v/100, v%100
	var parts []string
	if mt > 0 || ct == 0 {
		w := NumberWords(mt)
		switch {
		case mt == 1:
			w += " metical"
		case mt%1_000_000 == 0 && mt > 0:
			w += " de meticais" // "um milhão de meticais"
		default:
			w += " meticais"
		}
		parts = append(parts, w)
	}
	if ct > 0 {
		w := NumberWords(ct)
		if ct == 1 {
			w += " centavo"
		} else {
			w += " centavos"
		}
		parts = append(parts, w)
	}
	return strings.Join(parts, " e ")
}
