package main

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
	"github.com/pieropalacioscode/ladmincabo/internal/models"
	"github.com/pieropalacioscode/ladmincabo/internal/slug"
)

// brandAttrName: la columna "Marcas" (W) se guarda como atributo "Marca", igual que Proveedor.
const brandAttrName = "Marca"

func brandFromRow(raw string) (string, error) {
	v := normalizeProvider(raw)
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > 100 {
		return "", errors.New("la marca pasa de 100 caracteres")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", errors.New("la marca tiene caracteres de control")
		}
	}
	return v, nil
}

// blockHasAttr indica si alguno de los 16 bloques ya trae ese atributo con valor real
// (en ese caso manda el bloque y la columna Marcas no lo pisa).
func blockHasAttr(blocks [16]importer.Atributo, name string) bool {
	want := slug.Fold(name)
	for _, b := range blocks {
		if slug.Fold(strings.TrimSpace(b.Nombre)) == want && normalizeMaybeEmpty(b.Valor) != "" {
			return true
		}
	}
	return false
}

// upsertNamedAttribute guarda un atributo suelto (idempotente por producto y nombre).
func upsertNamedAttribute(db *gorm.DB, productID uint, name, value string, dryRun bool) (int, error) {
	if value == "" {
		return 0, nil
	}
	if dryRun {
		attrDiff.noteDry(db, productID, name, value)
		return 1, nil
	}
	var existing models.ProductAttribute
	err := db.Where("product_id = ? AND name = ?", productID, name).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := db.Create(&models.ProductAttribute{ProductID: productID, Name: name, Value: value}).Error; err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	case existing.Value != value:
		attrDiff.note(name, existing.Value, value)
		if keepsAttr(name) {
			return 1, nil
		}
		if err := db.Model(&existing).Update("value", value).Error; err != nil {
			return 0, err
		}
	}
	return 1, nil
}
