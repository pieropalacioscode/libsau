package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/models"
)

type SearchHandler struct {
	db *gorm.DB
}

func NewSearchHandler(db *gorm.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

// GET /api/v1/products/search?q=...
// Devuelve HTML para HTMX — no JSON.
// Búsqueda fuzzy con pg_trgm si está disponible, ILIKE como fallback.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var products []models.Product
	tx := h.db.Preload("Category").Where("active = ?", true)

	if q == "" {
		tx = tx.Order("id DESC").Limit(8)
	} else {
		tx = tx.Where(
			"name ILIKE ? OR sku ILIKE ?",
			"%"+q+"%", "%"+q+"%",
		).Limit(8)
	}

	if err := tx.Find(&products).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<p class="text-red-500 text-sm text-center">Error buscando productos</p>`)
		return
	}

	if len(products) == 0 {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w,
			`<p class="text-gray-400 text-sm text-center mt-8">No se encontró "%s"</p>`,
			html.EscapeString(q),
		)
		return
	}

	// Construir el HTML de los resultados
	var sb strings.Builder
	for _, p := range products {
		stockColor := "text-green-600"
		stockLabel := fmt.Sprintf("Stock: %d", p.Stock)
		disabled := ""
		onclick := fmt.Sprintf(
			"addToCart(%d, '%s', '%s', %.2f, %d)",
			p.ID,
			html.EscapeString(p.Name),
			html.EscapeString(p.SKU),
			p.Price,
			p.Stock,
		)

		if p.Stock == 0 {
			stockColor = "text-red-500"
			stockLabel = "Sin stock"
			disabled = "opacity-50 cursor-not-allowed"
			onclick = fmt.Sprintf(
				"beepError(); showSaleResult('Sin stock: %s', 'text-red-600')",
				html.EscapeString(p.Name),
			)
		} else if p.Stock <= 5 {
			stockColor = "text-orange-500"
		}

		sb.WriteString(fmt.Sprintf(`
		<div class="bg-white border border-gray-200 rounded-lg p-3 flex items-center
		            justify-between hover:border-blue-300 hover:shadow-sm transition %s">
		  <div class="flex-1 min-w-0 mr-3">
		    <p class="font-medium text-gray-800 text-sm truncate">%s</p>
		    <p class="text-gray-400 text-xs">%s · %s</p>
		    <p class="text-xs %s font-medium">%s</p>
		  </div>
		  <div class="flex items-center gap-3 flex-shrink-0">
		    <span class="font-bold text-blue-700 text-sm">S/%.2f</span>
		    <button onclick="%s"
		      class="bg-blue-700 hover:bg-blue-600 text-white text-xs
		             px-3 py-2 rounded-lg font-medium transition %s">
		      + Agregar
		    </button>
		  </div>
		</div>`,
			disabled,
			html.EscapeString(p.Name),
			html.EscapeString(p.SKU),
			html.EscapeString(p.Category.Name),
			stockColor,
			stockLabel,
			p.Price,
			onclick,
			disabled,
		))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sb.String())
}
