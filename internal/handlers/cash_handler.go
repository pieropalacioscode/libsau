package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/models"
)

type CashHandler struct {
	db *gorm.DB
}

func NewCashHandler(db *gorm.DB) *CashHandler {
	return &CashHandler{db: db}
}

type todayTotalsRow struct {
	SalesCount   int64
	TotalRevenue float64
	Cash         float64
	Yape         float64
	Plin         float64
	Tarjeta      float64
}

type TodayResponse struct {
	Date          string             `json:"date"`
	SalesCount    int64              `json:"sales_count"`
	Revenue       float64            `json:"revenue"`
	ByMethod      map[string]float64 `json:"by_method"`
	AlreadyClosed bool               `json:"already_closed"`
	Sales         []RecentSale       `json:"sales"`
}

type CloseRequest struct {
	CashDeclared float64 `json:"cash_declared"`
	Notes        string  `json:"notes"`
}

// GET /api/v1/cash/today
func (h *CashHandler) Today(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	date := start.Format("2006-01-02")

	var existing models.CashClose
	alreadyClosed := h.db.Where("date = ?", date).First(&existing).Error == nil

	var row todayTotalsRow
	h.db.Raw(`
		SELECT
			COUNT(*)                                                         AS sales_count,
			COALESCE(SUM(total), 0)                                          AS total_revenue,
			COALESCE(SUM(CASE WHEN pay_method='EFECTIVO' THEN total END), 0) AS cash,
			COALESCE(SUM(CASE WHEN pay_method='YAPE'     THEN total END), 0) AS yape,
			COALESCE(SUM(CASE WHEN pay_method='PLIN'     THEN total END), 0) AS plin,
			COALESCE(SUM(CASE WHEN pay_method='TARJETA'  THEN total END), 0) AS tarjeta
		FROM sales
		WHERE status      = 'COMPLETED'
		  AND created_at >= ?
		  AND created_at <  ?
		  AND deleted_at  IS NULL
	`, start, end).Scan(&row)

	var recentRows []recentSaleRow
	h.db.Raw(`
		SELECT s.id, s.total, s.pay_method, s.created_at,
		       COUNT(si.id) AS item_count,
		       u.name       AS seller
		FROM sales s
		LEFT JOIN sale_items si ON si.sale_id = s.id
		LEFT JOIN users u       ON u.id       = s.user_id
		WHERE s.status      = 'COMPLETED'
		  AND s.created_at >= ?
		  AND s.created_at <  ?
		  AND s.deleted_at  IS NULL
		GROUP BY s.id, u.name
		ORDER BY s.created_at DESC
	`, start, end).Scan(&recentRows)

	sales := make([]RecentSale, 0, len(recentRows))
	for _, r := range recentRows {
		sales = append(sales, RecentSale{
			ID:        r.ID,
			Total:     r.Total,
			PayMethod: r.PayMethod,
			ItemCount: r.ItemCount,
			Seller:    r.Seller,
			CreatedAt: r.CreatedAt,
		})
	}

	respondJSON(w, http.StatusOK, TodayResponse{
		Date:          date,
		SalesCount:    row.SalesCount,
		Revenue:       row.TotalRevenue,
		AlreadyClosed: alreadyClosed,
		ByMethod: map[string]float64{
			"EFECTIVO": row.Cash,
			"YAPE":     row.Yape,
			"PLIN":     row.Plin,
			"TARJETA":  row.Tarjeta,
		},
		Sales: sales,
	})
}

// POST /api/v1/cash/close
func (h *CashHandler) Close(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "no autenticado")
		return
	}

	var req CloseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	if req.CashDeclared < 0 {
		respondError(w, http.StatusBadRequest, "cash_declared no puede ser negativo")
		return
	}

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	date := start.Format("2006-01-02")

	var existing models.CashClose
	err := h.db.Where("date = ?", date).First(&existing).Error

	if err == nil {

		respondError(
			w,
			http.StatusConflict,
			"la caja del "+date+" ya fue cerrada",
		)

		return
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {

		respondError(w, http.StatusInternalServerError, "error verificando cierre")
		return
	}

	var row todayTotalsRow
	h.db.Raw(`
		SELECT
			COUNT(*)                                                         AS sales_count,
			COALESCE(SUM(total), 0)                                          AS total_revenue,
			COALESCE(SUM(CASE WHEN pay_method='EFECTIVO' THEN total END), 0) AS cash,
			COALESCE(SUM(CASE WHEN pay_method='YAPE'     THEN total END), 0) AS yape,
			COALESCE(SUM(CASE WHEN pay_method='PLIN'     THEN total END), 0) AS plin,
			COALESCE(SUM(CASE WHEN pay_method='TARJETA'  THEN total END), 0) AS tarjeta
		FROM sales
		WHERE status      = 'COMPLETED'
		  AND created_at >= ?
		  AND created_at <  ?
		  AND deleted_at  IS NULL
	`, start, end).Scan(&row)

	difference := req.CashDeclared - row.Cash

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "usuario no encontrado")
		return
	}

	cashClose := models.CashClose{
		Date:         date,
		UserID:       claims.UserID,
		UserName:     user.Name,
		SalesCount:   row.SalesCount,
		RevenueCalc:  row.TotalRevenue,
		CashCalc:     row.Cash,
		YapeCalc:     row.Yape,
		PlinCalc:     row.Plin,
		TarjetaCalc:  row.Tarjeta,
		CashDeclared: req.CashDeclared,
		Difference:   difference,
		Notes:        req.Notes,
		ClosedAt:     now,
	}
	cashClose.Hash = cashClose.ComputeHash()

	if err := h.db.Create(&cashClose).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error guardando cierre")
		return
	}

	alert := ""
	switch {
	case difference < -50:
		alert = "FALTANTE significativo. Revisar ventas del dia."
	case difference < 0:
		alert = "Faltante de " + formatMoney(difference) + ". Verificar."
	case difference > 50:
		alert = "Sobrante significativo. Revisar caja."
	case difference > 0:
		alert = "Sobrante de " + formatMoney(difference) + "."
	default:
		alert = "Caja cuadrada perfectamente."
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"message":   "caja cerrada correctamente",
		"date":      cashClose.Date,
		"closed_at": cashClose.ClosedAt,
		"summary": map[string]any{
			"sales_count":   cashClose.SalesCount,
			"revenue_total": cashClose.RevenueCalc,
			"by_method": map[string]float64{
				"EFECTIVO": cashClose.CashCalc,
				"YAPE":     cashClose.YapeCalc,
				"PLIN":     cashClose.PlinCalc,
				"TARJETA":  cashClose.TarjetaCalc,
			},
		},
		"cash": map[string]any{
			"calculated": cashClose.CashCalc,
			"declared":   cashClose.CashDeclared,
			"difference": cashClose.Difference,
		},
		"alert": alert,
		"hash":  cashClose.Hash,
	})
}

// GET /api/v1/cash/history
func (h *CashHandler) History(w http.ResponseWriter, r *http.Request) {
	var closes []models.CashClose
	h.db.Order("date DESC").Limit(30).Find(&closes)

	type closeWithStatus struct {
		models.CashClose
		HashOK bool `json:"hash_ok"`
	}

	result := make([]closeWithStatus, 0, len(closes))
	for _, c := range closes {
		result = append(result, closeWithStatus{
			CashClose: c,
			HashOK:    c.Hash == c.ComputeHash(),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  result,
		"total": len(result),
	})
}

func formatMoney(amount float64) string {
	if amount < 0 {
		return fmt.Sprintf("S/ -%.2f", -amount)
	}
	return fmt.Sprintf("S/ %.2f", amount)
}
