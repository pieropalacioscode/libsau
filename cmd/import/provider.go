package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
	"github.com/pieropalacioscode/ladmincabo/internal/slug"
)

// providerAttrName es el nombre con que la columna "Proveedor" (la 97 del Excel) se guarda como
// atributo del producto. Así la API pública la ofrece en /filtros y en /productos?f=Proveedor:X
// sin cambiar el esquema de la base de datos.
const providerAttrName = "Proveedor"

const providerMaxRunes = 100

// normalizeProvider recorta, colapsa los espacios internos y trata "-" como vacío. No cambia las
// mayúsculas: el nombre se publica como lo escribió el cliente.
func normalizeProvider(raw string) string {
	v := strings.Join(strings.Fields(raw), " ")
	if v == "-" {
		return ""
	}
	return v
}

// providerFromRow devuelve el proveedor listo para guardar ("" si la celda viene vacía).
func providerFromRow(raw string) (string, error) {
	v := normalizeProvider(raw)
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > providerMaxRunes {
		return "", fmt.Errorf("el proveedor pasa de %d caracteres", providerMaxRunes)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", errors.New("el proveedor tiene caracteres de control")
		}
	}
	return v, nil
}

// upsertProvider guarda el proveedor como atributo (idempotente por producto y nombre).
// Devuelve 1 si hay proveedor y 0 si no; en --dry-run no escribe.
func upsertProvider(db *gorm.DB, productID uint, value string, dryRun bool) (int, error) {
	if value == "" {
		return 0, nil
	}
	if dryRun {
		return 1, nil
	}
	var existing models.ProductAttribute
	err := db.Where("product_id = ? AND name = ?", productID, providerAttrName).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := db.Create(&models.ProductAttribute{ProductID: productID, Name: providerAttrName, Value: value}).Error; err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	case existing.Value != value:
		if err := db.Model(&existing).Update("value", value).Error; err != nil {
			return 0, err
		}
	}
	return 1, nil
}

// providerStats cuenta los productos por proveedor para mostrar un resumen al importar.
type providerStats struct{ counts map[string]int }

func newProviderStats() *providerStats { return &providerStats{counts: map[string]int{}} }

func (s *providerStats) add(name string) {
	if name == "" {
		name = "(sin proveedor)"
	}
	s.counts[name]++
}

// report imprime los proveedores de más a menos productos y avisa de los que solo se diferencian
// en mayúsculas o tildes, porque aparecerían como filas distintas en la tienda.
func (s *providerStats) report() {
	if len(s.counts) == 0 {
		return
	}
	type kv struct {
		name string
		n    int
	}
	list := make([]kv, 0, len(s.counts))
	for k, v := range s.counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].name < list[j].name
	})
	fmt.Println("Proveedores en este archivo:")
	for _, e := range list {
		fmt.Printf("   - %-28s %d\n", e.name, e.n)
	}
	for _, group := range caseVariants(s.counts) {
		fmt.Printf("⚠️  proveedores que solo difieren en mayúsculas o tildes: %s\n", strings.Join(group, " / "))
	}
}

// caseVariants devuelve los grupos de nombres que, sin mayúsculas ni tildes, son iguales.
func caseVariants(counts map[string]int) [][]string {
	by := map[string][]string{}
	for name := range counts {
		k := slug.Fold(name)
		by[k] = append(by[k], name)
	}
	var out [][]string
	for _, names := range by {
		if len(names) > 1 {
			sort.Strings(names)
			out = append(out, names)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
