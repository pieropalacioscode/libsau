package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeProvider(t *testing.T) {
	cases := map[string]string{
		"  ProveedorA ": "ProveedorA",
		"-":            "",
		"":             "",
		"Mily   Books": "Mily Books",
		"\tÁlvaro\n":   "Álvaro",
	}
	for in, want := range cases {
		if got := normalizeProvider(in); got != want {
			t.Errorf("normalizeProvider(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestProviderFromRow(t *testing.T) {
	if v, err := providerFromRow(" ProveedorB "); err != nil || v != "ProveedorB" {
		t.Errorf("caso válido: %q, %v", v, err)
	}
	if v, err := providerFromRow("-"); err != nil || v != "" {
		t.Errorf("el guion debe ser vacío y sin error: %q, %v", v, err)
	}
	if _, err := providerFromRow(strings.Repeat("a", 101)); err == nil {
		t.Error("más de 100 caracteres debe fallar")
	}
	if _, err := providerFromRow("Ab\x01c"); err == nil {
		t.Error("un carácter de control debe fallar")
	}
	if _, err := providerFromRow(strings.Repeat("á", 100)); err != nil {
		t.Errorf("100 caracteres (no bytes) debe pasar: %v", err)
	}
}

func TestCaseVariants(t *testing.T) {
	got := caseVariants(map[string]int{"ProveedorA": 2, "proveedora": 1, "ProveedorC": 3, "Álvaro": 1, "Alvaro": 1})
	if want := "[[Alvaro Álvaro] [ProveedorA proveedora]]"; fmt.Sprint(got) != want {
		t.Errorf("caseVariants = %v, quería %s", got, want)
	}
	if len(caseVariants(map[string]int{"A": 1, "B": 2})) != 0 {
		t.Error("nombres distintos no son variantes")
	}
}

func TestProviderStatsSinProveedor(t *testing.T) {
	s := newProviderStats()
	s.add("")
	s.add("ProveedorC")
	s.add("")
	if s.counts["(sin proveedor)"] != 2 || s.counts["ProveedorC"] != 1 {
		t.Errorf("conteos inesperados: %v", s.counts)
	}
}
