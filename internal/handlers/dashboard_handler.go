package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/models"
)

// loc se inicializa una sola vez al arrancar el proceso.
// Si la zona horaria no existe en el sistema, el servidor no debe arrancar.
var loc = func() *time.Location {
	l, err := time.LoadLocation("America/Lima")
	if err != nil {
		panic("no se pudo cargar la zona horaria America/Lima: " + err.Error())
	}
	return l
}()

// ─── Handler ──────────────────────────────────────────────────────────────────

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

// ─── Structs de respuesta ─────────────────────────────────────────────────────

type DashboardResponse struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Today       SalesMetrics   `json:"today"`
	LowStock    []LowStockItem `json:"low_stock"`
	RecentSales []RecentSale   `json:"recent_sales"`
}

type SalesMetrics struct {
	Date         string             `json:"date"`
	TotalSales   int64              `json:"total_sales"`
	TotalRevenue float64            `json:"total_revenue"`
	TotalProfit  float64            `json:"total_profit"`
	ByPayMethod  map[string]float64 `json:"by_pay_method"`
}

type LowStockItem struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	SKU      string `json:"sku"`
	Stock    int    `json:"stock"`
	Category string `json:"category"`
}

// ─── Structs para queries raw ─────────────────────────────────────────────────

type payMethodRow struct {
	PayMethod string
	Revenue   float64
}

type dailyTotalsRow struct {
	TotalSales   int64
	TotalRevenue float64
	TotalProfit  float64
}

// ─── GET /api/v1/dashboard ────────────────────────────────────────────────────

func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	// "Ahora" en hora peruana — esto determina qué día es "hoy" para el negocio.
	now := time.Now().In(loc)

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	log.Println("DEBUG DATE:", startOfDay, "→", endOfDay)

	response := DashboardResponse{
		GeneratedAt: now.UTC(),
	}

	// ── Query 1: Totales del día ──────────────────────────────────────────────
	var totals dailyTotalsRow
	h.db.Raw(`
		SELECT
			COUNT(s.id)                                      AS total_sales,
			COALESCE(SUM(s.total), 0)                        AS total_revenue,
			COALESCE(SUM(
				(si.price_snapshot - si.cost_snapshot) * si.quantity - si.discount
			), 0)                                            AS total_profit
		FROM sales s
		LEFT JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'COMPLETED'
		  AND s.created_at >= ?
		  AND s.created_at <  ?
		  AND s.deleted_at IS NULL
	`, startOfDay, endOfDay).Scan(&totals)

	// ── Query 2: Desglose por método de pago ─────────────────────────────────
	var payRows []payMethodRow
	h.db.Raw(`
		SELECT pay_method, COALESCE(SUM(total), 0) AS revenue
		FROM sales
		WHERE status     = 'COMPLETED'
		  AND created_at >= ?
		  AND created_at <  ?
		  AND deleted_at IS NULL
		GROUP BY pay_method
	`, startOfDay, endOfDay).Scan(&payRows)

	byPayMethod := make(map[string]float64)
	for _, row := range payRows {
		byPayMethod[row.PayMethod] = row.Revenue
	}
	// Garantizar que los 4 métodos siempre aparecen aunque valgan 0.
	for _, method := range []string{"EFECTIVO", "YAPE", "PLIN", "TARJETA"} {
		if _, exists := byPayMethod[method]; !exists {
			byPayMethod[method] = 0
		}
	}

	response.Today = SalesMetrics{
		Date:         startOfDay.Format("2006-01-02"),
		TotalSales:   totals.TotalSales,
		TotalRevenue: totals.TotalRevenue,
		TotalProfit:  totals.TotalProfit,
		ByPayMethod:  byPayMethod,
	}

	// ── Query 3: Stock bajo ───────────────────────────────────────────────────
	threshold := 5
	if t := r.URL.Query().Get("low_stock_threshold"); t != "" {
		if parsed, err := parsePositiveInt(t); err == nil {
			threshold = parsed
		}
	}

	var lowStockProducts []models.Product
	h.db.Preload("Category").
		Where("stock <= ? AND active = ?", threshold, true).
		Order("stock ASC").
		Limit(10).
		Find(&lowStockProducts)

	for _, p := range lowStockProducts {
		response.LowStock = append(response.LowStock, LowStockItem{
			ID:       p.ID,
			Name:     p.Name,
			SKU:      p.SKU,
			Stock:    p.Stock,
			Category: p.Category.Name,
		})
	}
	if response.LowStock == nil {
		response.LowStock = []LowStockItem{}
	}

	// ── Query 4: Últimas 5 ventas ─────────────────────────────────────────────
	var recentRows []recentSaleRow
	h.db.Raw(`
		SELECT
			s.id,
			s.total,
			s.pay_method,
			s.created_at,
			COUNT(si.id) AS item_count,
			u.name       AS seller
		FROM sales s
		LEFT JOIN sale_items si ON si.sale_id = s.id
		LEFT JOIN users u       ON u.id       = s.user_id
		WHERE s.deleted_at IS NULL
		GROUP BY s.id, u.name
		ORDER BY s.created_at DESC
		LIMIT 5
	`).Scan(&recentRows)

	for _, row := range recentRows {
		response.RecentSales = append(response.RecentSales, RecentSale{
			ID:        row.ID,
			Total:     row.Total,
			PayMethod: row.PayMethod,
			ItemCount: row.ItemCount,
			Seller:    row.Seller,
			CreatedAt: row.CreatedAt,
		})
	}
	if response.RecentSales == nil {
		response.RecentSales = []RecentSale{}
	}

	respondJSON(w, http.StatusOK, response)
}

// parsePositiveInt parsea un string a int positivo.
func parsePositiveInt(s string) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("no es número")
		}
		v = v*10 + int(c-'0')
	}
	if v <= 0 {
		return 0, fmt.Errorf("inválido")
	}
	return v, nil
}
