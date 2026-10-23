package main

import (
	"testing"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

func TestAttrStats(t *testing.T) {
	s := newAttrStats()
	var b [16]importer.Atributo
	for _, v := range []string{"A color", "a color", "-"} {
		b[0] = importer.Atributo{Nombre: "Impresión", Valor: v}
		s.add(b)
	}
	b[0] = importer.Atributo{Nombre: "ISBN", Valor: "123"}
	s.add(b)
	st := s.by["Impresión"]
	if st == nil || st.n != 2 || st.empty != 1 || len(st.values) != 2 {
		t.Fatalf("conteos inesperados: %+v", st)
	}
	if _, ok := s.by["ISBN"]; ok {
		t.Error("el atributo ISBN ya vive en otra parte y no debe contarse")
	}
	if v := s.variants("Impresión"); len(v) != 1 || len(v[0]) != 2 {
		t.Errorf("debía detectar la variante de mayúsculas: %v", v)
	}
}

func TestBlockHasAttrYBrand(t *testing.T) {
	var b [16]importer.Atributo
	b[3] = importer.Atributo{Nombre: " marca ", Valor: "SM"}
	if !blockHasAttr(b, "Marca") {
		t.Error("un bloque con valor real debe contar")
	}
	b[3].Valor = "-"
	if blockHasAttr(b, "Marca") {
		t.Error("un bloque con «-» no cuenta")
	}
	if v, err := brandFromRow("  Lo   que Leo "); err != nil || v != "Lo que Leo" {
		t.Errorf("brandFromRow: %q, %v", v, err)
	}
	if v, _ := brandFromRow("-"); v != "" {
		t.Error("«-» es vacío")
	}
	if _, err := brandFromRow("Ab\x01c"); err == nil {
		t.Error("un carácter de control debe fallar")
	}
}
