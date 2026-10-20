package main

import (
	"strings"
	"testing"
)

func TestCleanISBN(t *testing.T) {
	cases := []struct {
		in        string
		want      string
		converted bool
	}{
		{"9780306406157", "9780306406157", false},
		{"978-0-306-40615-7", "9780306406157", false},
		{" 978 0306406157 ", "9780306406157", false},
		{"9780306406158", "", false},
		{"0306406152", "9780306406157", true},
		{"080442957x", "9780804429573", true},
		{"0306406153", "", false},
		{"5901234123457", "", false},
		{"0000000000000", "", false},
		{"96385074", "96385074", false},
		{"036000291452", "036000291452", false},
		{"978997200712x", "", false},
		{"978997223200", "", false},
		{"12345", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, note := cleanISBN(c.in)
		if got != c.want {
			t.Errorf("cleanISBN(%q) = %q, quería %q (%s)", c.in, got, c.want, note)
		}
		if conv := note == "convertido de ISBN-10"; conv != c.converted {
			t.Errorf("cleanISBN(%q): convertido = %v, quería %v", c.in, conv, c.converted)
		}
		if got == "" && note == "" {
			t.Errorf("cleanISBN(%q): descartado sin explicar por qué", c.in)
		}
	}
}

func TestISBN10To13(t *testing.T) {
	if got := isbn10To13("0306406152"); got != "9780306406157" {
		t.Errorf("isbn10To13 = %s", got)
	}
	if got := isbn10To13("080442957X"); got != "9780804429573" {
		t.Errorf("isbn10To13 = %s", got)
	}
}

func TestGuardRepetidoEnElArchivo(t *testing.T) {
	g := newISBNGuard(false, nil)
	if v, w := g.decide("A-1", "9780306406157", ""); v != "9780306406157" || w != "" {
		t.Fatalf("el primero debe conservarlo: %q %q", v, w)
	}
	if v, w := g.decide("B-2", "", "9780306406157"); v != "" || !strings.Contains(w, "A-1") {
		t.Errorf("el segundo debe quedar sin ISBN y avisar del primero: %q %q", v, w)
	}
	if v, _ := g.decide("A-1", "9780306406157", ""); v != "9780306406157" {
		t.Errorf("el mismo SKU no es un repetido: %q", v)
	}
	if g.repeated != 1 {
		t.Errorf("repetidos = %d, quería 1", g.repeated)
	}
}

func TestGuardRepetidoContraLaBase(t *testing.T) {
	g := newISBNGuard(false, func(isbn, sku string) string {
		if isbn == "9780306406157" && sku != "OTRO-9" {
			return "OTRO-9"
		}
		return ""
	})
	if v, w := g.decide("NUEVO-1", "9780306406157", ""); v != "" || !strings.Contains(w, "OTRO-9") {
		t.Errorf("debe avisar del SKU de la base: %q %q", v, w)
	}
	if v, w := g.decide("OTRO-9", "9780306406157", ""); v != "9780306406157" || w != "" {
		t.Errorf("el dueño en la base conserva su ISBN: %q %q", v, w)
	}
}

func TestGuardEstricto(t *testing.T) {
	g := newISBNGuard(true, nil)
	if v, w := g.decide("S1", "5901234123457", ""); v != "" || w == "" {
		t.Errorf("el código de ejemplo debe descartarse con aviso: %q %q", v, w)
	}
	if v, w := g.decide("S2", "0306406152", ""); v != "9780306406157" || w != "" {
		t.Errorf("el ISBN-10 válido debe convertirse: %q %q", v, w)
	}
	if g.invalid != 1 || g.converted != 1 {
		t.Errorf("contadores: inválidos=%d convertidos=%d", g.invalid, g.converted)
	}
}

func TestGuardSinEstrictoNoTocaElFormato(t *testing.T) {
	g := newISBNGuard(false, nil)
	if v, w := g.decide("S1", "5901234123457", ""); v != "5901234123457" || w != "" {
		t.Errorf("sin --strict-isbn se conserva como hoy: %q %q", v, w)
	}
	if v, w := g.decide("S2", "-", ""); v != "" || w != "" {
		t.Errorf("el guion es vacío y sin aviso: %q %q", v, w)
	}
	if v, w := g.decide("S3", strings.Repeat("1", 21), ""); v != "" || w == "" {
		t.Errorf("más de 20 caracteres no cabe en la columna: %q %q", v, w)
	}
}
