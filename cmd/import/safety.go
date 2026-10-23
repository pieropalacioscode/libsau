package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
	"github.com/pieropalacioscode/ladmincabo/internal/models"
	"github.com/pieropalacioscode/ladmincabo/internal/slug"
)

// importOpts son las opciones de protección de la importación (se llenan en main).
var importOpts struct {
	keepPrices bool            // no actualizar precios de productos que ya existen
	keepAttrs  map[string]bool // atributos cuyo valor ya guardado no se pisa (nombre sin tildes ni mayúsculas)
}

func setKeepAttrs(csv string) {
	importOpts.keepAttrs = map[string]bool{}
	for _, n := range strings.Split(csv, ",") {
		if n = strings.TrimSpace(n); n != "" {
			importOpts.keepAttrs[slug.Fold(n)] = true
		}
	}
}

func keepsAttr(name string) bool { return importOpts.keepAttrs[slug.Fold(name)] }

// buildProductUpdates arma los campos que una reimportación actualiza en un producto que ya existe.
// Una celda de ISBN vacía NUNCA borra el ISBN guardado, y con keepPrices el precio no se toca.
func buildProductUpdates(row importer.ProductRow, categoryID uint, imageURL, description *string, active, respectPublished, keepPrices bool) map[string]any {
	u := map[string]any{
		"name":        row.Nombre,
		"stock":       row.Inventario,
		"category_id": categoryID,
	}
	if v := isbnFromRow(row); v != nil {
		u["isbn"] = *v
	}
	if !keepPrices {
		u["price"] = row.PrecioNormal
	}
	if imageURL != nil {
		u["image_url"] = *imageURL
	}
	if description != nil {
		u["description"] = *description
	}
	if respectPublished {
		u["active"] = active
	}
	return u
}

// ── Informe de precios ───────────────────────────────────────────────────────

type priceChange struct {
	SKU      string
	Old, New float64
}

type priceChanges struct {
	list []priceChange
	same int
}

var priceReport = &priceChanges{}

func (p *priceChanges) note(sku string, oldP, newP float64) {
	if math.Abs(oldP-newP) < 0.005 {
		p.same++
		return
	}
	p.list = append(p.list, priceChange{sku, oldP, newP})
}

func (p *priceChanges) top(n int) []priceChange {
	out := append([]priceChange(nil), p.list...)
	sort.Slice(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].New-out[i].Old), math.Abs(out[j].New-out[j].Old)
		if di != dj {
			return di > dj
		}
		return out[i].SKU < out[j].SKU
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (p *priceChanges) report(keep, dry bool) {
	if len(p.list) == 0 && p.same == 0 {
		return
	}
	up := 0
	for _, c := range p.list {
		if c.New > c.Old {
			up++
		}
	}
	down := len(p.list) - up
	switch {
	case keep && len(p.list) > 0:
		fmt.Printf("💲 --keep-prices: %d precio(s) distintos en el Excel NO se actualizan; %d sin cambio\n", len(p.list), p.same)
	case dry:
		fmt.Printf("💲 Precios de productos existentes: %d cambiarían (%d suben, %d bajan), %d sin cambio\n", len(p.list), up, down, p.same)
	default:
		fmt.Printf("💲 Precios de productos existentes: %d actualizados (%d suben, %d bajan), %d sin cambio\n", len(p.list), up, down, p.same)
	}
	for _, c := range p.top(10) {
		fmt.Printf("   - %-36s S/ %.2f → S/ %.2f\n", c.SKU, c.Old, c.New)
	}
	if len(p.list) > 10 {
		fmt.Printf("   … y %d más\n", len(p.list)-10)
	}
}

// ── Informe de atributos que cambian de valor ────────────────────────────────

type attrChanges struct{ byAttr map[string]map[string]int }

var attrDiff = &attrChanges{byAttr: map[string]map[string]int{}}

func (a *attrChanges) note(name, oldV, newV string) {
	m := a.byAttr[name]
	if m == nil {
		m = map[string]int{}
		a.byAttr[name] = m
	}
	m[fmt.Sprintf("«%s» → «%s»", oldV, newV)]++
}

// noteDry compara con la base en el ensayo (en la importación real se anota al actualizar).
func (a *attrChanges) noteDry(db *gorm.DB, productID uint, name, value string) {
	if productID == 0 {
		return
	}
	var ex models.ProductAttribute
	if err := db.Where("product_id = ? AND name = ?", productID, name).First(&ex).Error; err != nil {
		return
	}
	if ex.Value != value {
		a.note(name, ex.Value, value)
	}
}

func (a *attrChanges) report() {
	if len(a.byAttr) == 0 {
		fmt.Println("🏷️  Atributos de productos existentes: sin cambios de valor")
		return
	}
	type row struct {
		name  string
		total int
		moves map[string]int
	}
	rows := make([]row, 0, len(a.byAttr))
	for n, m := range a.byAttr {
		t := 0
		for _, c := range m {
			t += c
		}
		rows = append(rows, row{n, t, m})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].total != rows[j].total {
			return rows[i].total > rows[j].total
		}
		return rows[i].name < rows[j].name
	})
	fmt.Println("🏷️  Atributos de productos existentes que cambian de valor:")
	for _, r := range rows {
		note := ""
		if keepsAttr(r.name) {
			note = "  (se conservan: --keep-attrs)"
		}
		fmt.Printf("   - %s: %d producto(s)%s\n", r.name, r.total, note)
		keys := make([]string, 0, len(r.moves))
		for k := range r.moves {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if r.moves[keys[i]] != r.moves[keys[j]] {
				return r.moves[keys[i]] > r.moves[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for i, k := range keys {
			if i >= 3 {
				break
			}
			fmt.Printf("       %s ×%d\n", k, r.moves[k])
		}
	}
}
