package money

import "testing"

func TestNumberWords(t *testing.T) {
	cases := map[int64]string{
		0:          "zero",
		1:          "um",
		16:         "dezasseis",
		21:         "vinte e um",
		100:        "cem",
		101:        "cento e um",
		265:        "duzentos e sessenta e cinco",
		1000:       "mil",
		1001:       "mil e um",
		1100:       "mil e cem",
		1250:       "mil duzentos e cinquenta",
		2800:       "dois mil e oitocentos",
		5035:       "cinco mil e trinta e cinco",
		50350:      "cinquenta mil trezentos e cinquenta",
		100000:     "cem mil",
		1000000:    "um milhão",
		2000500:    "dois milhões e quinhentos",
		1250000:    "um milhão duzentos e cinquenta mil",
		3000000000: "três mil milhões",
	}
	for n, want := range cases {
		if got := NumberWords(n); got != want {
			t.Errorf("%d: got %q want %q", n, got, want)
		}
	}
}

func TestAmountWords(t *testing.T) {
	cases := map[Amount]string{
		MT(1):         "um metical",
		MT(5035):      "cinco mil e trinta e cinco meticais",
		Amount(13333): "cento e trinta e três meticais e trinta e três centavos",
		Amount(50):    "cinquenta centavos",
		0:             "zero meticais",
		MT(1000000):   "um milhão de meticais",
	}
	for a, want := range cases {
		if got := a.Words(); got != want {
			t.Errorf("%d: got %q want %q", a, got, want)
		}
	}
}
