package slug

import "testing"

func TestForCategory(t *testing.T) {
	cases := map[string]string{
		"Ciencia para niños": "ciencia-para-ninos",
		"Literatura clásica": "literatura-clasica",
		"  Mitos y leyendas": "mitos-y-leyendas",
		"2024":               "2024",
		"":                   "categoria",
		"¡¡¡":                "categoria",
	}
	for in, want := range cases {
		if got := ForCategory(in); got != want {
			t.Errorf("ForCategory(%q) = %q, quería %q", in, got, want)
		}
	}
}
