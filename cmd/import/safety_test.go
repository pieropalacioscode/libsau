package main

import (
	"testing"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

func TestBuildProductUpdates(t *testing.T) {
	row := importer.ProductRow{Nombre: "Libro", PrecioNormal: 12.5}
	u := buildProductUpdates(row, 7, nil, nil, true, false, false)
	if _, ok := u["isbn"]; ok {
		t.Error("una celda de ISBN vacía no debe borrar el ISBN guardado")
	}
	if u["price"] != 12.5 || u["category_id"] != uint(7) || u["name"] != "Libro" {
		t.Errorf("campos básicos inesperados: %v", u)
	}
	for _, k := range []string{"image_url", "description", "active"} {
		if _, ok := u[k]; ok {
			t.Errorf("%s no debía actualizarse", k)
		}
	}

	row.GTIN = "9786123456789"
	if u := buildProductUpdates(row, 1, nil, nil, true, false, false); u["isbn"] != "9786123456789" {
		t.Errorf("un ISBN con valor sí debe actualizarse: %v", u["isbn"])
	}
	if u := buildProductUpdates(row, 1, nil, nil, true, false, true); u["price"] != nil {
		t.Error("con keepPrices el precio no debe tocarse")
	}

	img, desc := "libprep/a.avif", "texto"
	u = buildProductUpdates(row, 1, &img, &desc, false, true, false)
	if u["image_url"] != img || u["description"] != desc || u["active"] != false {
		t.Errorf("imagen, descripción y activo: %v", u)
	}
}

func TestPriceChanges(t *testing.T) {
	p := &priceChanges{}
	p.note("A", 10, 10.001)
	p.note("B", 10, 12)
	p.note("C", 20, 5)
	if p.same != 1 || len(p.list) != 2 {
		t.Fatalf("same=%d list=%d", p.same, len(p.list))
	}
	if top := p.top(1); len(top) != 1 || top[0].SKU != "C" {
		t.Errorf("la mayor diferencia debe ir primero: %v", top)
	}
}

func TestKeepsAttr(t *testing.T) {
	setKeepAttrs("Impresión, Editorial")
	defer setKeepAttrs("")
	if !keepsAttr("impresion") || !keepsAttr("EDITORIAL") || keepsAttr("Marca") {
		t.Error("keepsAttr debe ignorar tildes y mayúsculas")
	}
}
