package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/middleware"
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
// Lista ventas WOO pendientes de confirmación DEL NEGOCIO ACTUAL.
func (h *WooConfirmHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	bid := middleware.BusinessID(r)

	var sales []models.Sale
	if err := h.db.
		Preload("Items.Product").
		Where("business_id = ? AND origin = ? AND status = ?", bid, "WOO", "PENDING_CONFIRM").
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
// Solo actúa sobre pedidos del negocio actual: uno ajeno responde 404.
func (h *WooConfirmHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "no autenticado")
		return
	}
	bid := middleware.BusinessID(r)

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
		// Confirmar: SELECT FOR UPDATE + descontar stock (todo dentro del negocio)
		if err := h.worker.ConfirmSale(uint(id), claims.UserID, bid); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondError(w, http.StatusNotFound, "pedido no encontrado")
				return
			}
			respondError(w, http.StatusConflict, err.Error())
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

	// sales.notes es varchar(300): se recorta el motivo para no romper el UPDATE
	reason := req.Reason
	if rs := []rune(reason); len(rs) > 250 {
		reason = string(rs[:250])
	}
	note := "Rechazado: " + reason

	res := h.db.Model(&models.Sale{}).
		Where("id = ? AND business_id = ? AND status = ?", id, bid, "PENDING_CONFIRM").
		Updates(map[string]any{
			"status": "CANCELLED",
			"notes":  note,
		})
	if res.Error != nil {
		respondError(w, http.StatusInternalServerError, "error rechazando pedido")
		return
	}
	if res.RowsAffected == 0 {
		respondError(w, http.StatusNotFound, "pedido pendiente no encontrado")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "pedido rechazado",
		"status":  "CANCELLED",
	})
}
