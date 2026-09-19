package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/models"
	"github.com/neocode96/libsau/internal/webhook"
)

type WooConfirmHandler struct {
	db     *gorm.DB
	worker *webhook.WooWorker
}

func NewWooConfirmHandler(db *gorm.DB, worker *webhook.WooWorker) *WooConfirmHandler {
	return &WooConfirmHandler{db: db, worker: worker}
}

// GET /api/v1/sales/pending
// Lista ventas WOO pendientes de confirmación.
func (h *WooConfirmHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	var sales []models.Sale
	if err := h.db.
		Preload("Items.Product").
		Where("origin = ? AND status = ?", "WOO", "PENDING_CONFIRM").
		Order("created_at ASC"). // más antiguas primero
		Find(&sales).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error listando pedidos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  sales,
		"total": len(sales),
	})
}

type ConfirmRequest struct {
	Action string `json:"action"` // "confirm" | "reject"
	Reason string `json:"reason"` // obligatorio si action = "reject"
}

// PATCH /api/v1/sales/:id/confirm
func (h *WooConfirmHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "no autenticado")
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req ConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if req.Action != "confirm" && req.Action != "reject" {
		respondError(w, http.StatusBadRequest, "action debe ser 'confirm' o 'reject'")
		return
	}

	if req.Action == "confirm" {
		// Confirmar: SELECT FOR UPDATE + descontar stock
		if err := h.worker.ConfirmSale(uint(id), claims.UserID); err != nil {
			if err.Error() != "" {
				respondError(w, http.StatusConflict, err.Error())
				return
			}
			respondError(w, http.StatusInternalServerError, "error confirmando venta")
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{
			"message": "pedido confirmado y stock descontado",
			"status":  "COMPLETED",
		})
		return
	}

	// Rechazar: marcar como CANCELLED sin tocar stock
	if req.Reason == "" {
		respondError(w, http.StatusBadRequest, "reason es obligatorio para rechazar")
		return
	}

	note := "Rechazado: " + req.Reason
	if err := h.db.Model(&models.Sale{}).Where("id = ? AND status = ?", id, "PENDING_CONFIRM").
		Updates(map[string]any{
			"status": "CANCELLED",
			"notes":  note,
		}).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error rechazando pedido")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "pedido rechazado",
		"status":  "CANCELLED",
	})
}
