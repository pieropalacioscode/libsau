package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/neocode96/libsau/internal/middleware"
	"github.com/neocode96/libsau/internal/models"
)

type ProductHandler struct {
	db *gorm.DB
}

func NewProductHandler(db *gorm.DB) *ProductHandler {
	return &ProductHandler{db: db}
}

type CreateProductRequest struct {
	Name       string  `json:"name"`
	SKU        string  `json:"sku"`
	Price      float64 `json:"price"`
	Cost       float64 `json:"cost"`
	Stock      int     `json:"stock"`
	CategoryID uint    `json:"category_id"`
}

type UpdateProductRequest struct {
	Name       *string  `json:"name"`
	Price      *float64 `json:"price"`
	Cost       *float64 `json:"cost"`
	CategoryID *uint    `json:"category_id"`
}

type AdjustStockRequest struct {
	Tipo   string `json:"tipo"`
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

// ── Helpers compartidos por todo el package handlers ─────────────────────────

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}

func pathID(r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	return uint(id), err == nil && id > 0
}

// ── GET /products ────────────────────────────────────────────────────────────

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	var products []models.Product
	if err := h.db.
		Where("business_id = ? AND active = ?", middleware.BusinessID(r), true).
		Preload("Category").
		Find(&products).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo productos")
		return
	}
	respondJSON(w, http.StatusOK, products)
}

// ── POST /products ───────────────────────────────────────────────────────────

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.SKU = strings.TrimSpace(req.SKU)
	switch {
	case req.Name == "":
		respondError(w, http.StatusBadRequest, "name es requerido")
		return
	case req.SKU == "":
		respondError(w, http.StatusBadRequest, "sku es requerido")
		return
	case req.CategoryID == 0:
		respondError(w, http.StatusBadRequest, "category_id es requerido")
		return
	case req.Price < 0 || req.Cost < 0 || req.Stock < 0:
		respondError(w, http.StatusBadRequest, "price, cost y stock no pueden ser negativos")
		return
	}

	bid := middleware.BusinessID(r)

	// La categoría debe pertenecer al mismo negocio que el producto.
	var category models.Category
	if err := h.db.Where("id = ? AND business_id = ?", req.CategoryID, bid).First(&category).Error; err != nil {
		respondError(w, http.StatusBadRequest, "categoría no existe en este negocio")
		return
	}

	product := models.Product{
		BusinessID: bid,
		Name:       req.Name,
		SKU:        req.SKU,
		Price:      req.Price,
		Cost:       req.Cost,
		Stock:      req.Stock,
		CategoryID: req.CategoryID,
		Active:     true,
	}
	if err := h.db.Create(&product).Error; err != nil {
		if isDuplicateKey(err) {
			respondError(w, http.StatusConflict, "ya existe un producto con ese SKU en este negocio")
			return
		}
		respondError(w, http.StatusInternalServerError, "error guardando producto")
		return
	}

	h.db.Preload("Category").First(&product, product.ID)
	respondJSON(w, http.StatusCreated, product)
}

// ── GET /products/{id} ───────────────────────────────────────────────────────

func (h *ProductHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var product models.Product
	err := h.db.Preload("Category").
		Where("id = ? AND business_id = ?", id, middleware.BusinessID(r)).
		First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error buscando producto")
		return
	}
	respondJSON(w, http.StatusOK, product)
}

// ── PATCH /products/{id} ─────────────────────────────────────────────────────

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req UpdateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	bid := middleware.BusinessID(r)

	var product models.Product
	if err := h.db.Where("id = ? AND business_id = ?", id, bid).First(&product).Error; err != nil {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			respondError(w, http.StatusBadRequest, "name vacío")
			return
		}
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Price != nil {
		if *req.Price < 0 {
			respondError(w, http.StatusBadRequest, "price inválido")
			return
		}
		updates["price"] = *req.Price
	}
	if req.Cost != nil {
		if *req.Cost < 0 {
			respondError(w, http.StatusBadRequest, "cost inválido")
			return
		}
		updates["cost"] = *req.Cost
	}
	if req.CategoryID != nil {
		var c models.Category
		if err := h.db.Where("id = ? AND business_id = ?", *req.CategoryID, bid).First(&c).Error; err != nil {
			respondError(w, http.StatusBadRequest, "categoría no existe en este negocio")
			return
		}
		updates["category_id"] = *req.CategoryID
	}

	if len(updates) == 0 {
		respondError(w, http.StatusBadRequest, "nada que actualizar")
		return
	}

	if err := h.db.Model(&product).Updates(updates).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error actualizando producto")
		return
	}

	h.db.Preload("Category").First(&product, product.ID)
	respondJSON(w, http.StatusOK, product)
}

// ── DELETE /products/{id} (soft: active = false) ─────────────────────────────

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var product models.Product
	if err := h.db.Where("id = ? AND business_id = ?", id, middleware.BusinessID(r)).First(&product).Error; err != nil {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if !product.Active {
		respondError(w, http.StatusConflict, "ya está eliminado")
		return
	}

	if err := h.db.Model(&product).Update("active", false).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error eliminando producto")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "producto eliminado"})
}

// ── PATCH /products/{id}/stock ───────────────────────────────────────────────

func (h *ProductHandler) AdjustStock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req AdjustStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Amount <= 0 {
		respondError(w, http.StatusBadRequest, "amount inválido")
		return
	}
	if len(req.Reason) < 5 {
		respondError(w, http.StatusBadRequest, "reason requerido")
		return
	}

	bid := middleware.BusinessID(r)

	var product models.Product
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Lock: dos ajustes simultáneos ya no se pisan (antes: Save sobre lectura sin lock).
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", id, bid).
			First(&product).Error; err != nil {
			return err
		}

		after := product.Stock
		switch req.Tipo {
		case "set":
			after = req.Amount
		case "add":
			after = product.Stock + req.Amount
		case "sub":
			if product.Stock < req.Amount {
				return fmt.Errorf("stock insuficiente")
			}
			after = product.Stock - req.Amount
		default:
			return fmt.Errorf("tipo inválido")
		}

		return tx.Model(&product).UpdateColumn("stock", after).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, "producto no encontrado")
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.db.Preload("Category").First(&product, product.ID)
	respondJSON(w, http.StatusOK, product)
}
