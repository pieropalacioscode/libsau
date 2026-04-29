package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/neocode96/libsau/internal/models"
	"gorm.io/gorm"
)

type ProductHandler struct {
	db *gorm.DB
}

func NewProductHandler(db *gorm.DB) *ProductHandler {
	return &ProductHandler{db: db}
}

// ─────────────────────────────────────────────
// DTO (entrada limpia)
type CreateProductRequest struct {
	Name       string  `json:"name"`
	SKU        string  `json:"sku"`
	Price      float64 `json:"price"`
	Stock      int     `json:"stock"`
	CategoryID uint    `json:"category_id"`
}

// ─────────────────────────────────────────────
// Helpers

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// ─────────────────────────────────────────────
// GET /products

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	var products []models.Product

	if err := h.db.
		Where("active = ?", true).
		Preload("Category").
		Find(&products).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo productos")
		return
	}

	respondJSON(w, http.StatusOK, products)
}

// ─────────────────────────────────────────────
// POST /products

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest

	// Decode JSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	// Validaciones mínimas
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name es requerido")
		return
	}
	if req.SKU == "" {
		respondError(w, http.StatusBadRequest, "sku es requerido")
		return
	}
	if req.CategoryID == 0 {
		respondError(w, http.StatusBadRequest, "category_id es requerido")
		return
	}

	// Verificar que la categoría existe
	var category models.Category
	if err := h.db.First(&category, req.CategoryID).Error; err != nil {
		respondError(w, http.StatusBadRequest, "categoría no existe")
		return
	}

	// Crear modelo
	product := models.Product{
		Name:       req.Name,
		SKU:        req.SKU,
		Price:      req.Price,
		Stock:      req.Stock,
		CategoryID: req.CategoryID,
	}

	if err := h.db.Create(&product).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error guardando producto")
		return
	}

	// Cargar relación
	h.db.Preload("Category").First(&product, product.ID)

	respondJSON(w, http.StatusCreated, product)
}

// ─────────────────────────────────────────────
// GET /products/{id}

func (h *ProductHandler) Get(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")

	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var product models.Product

	if err := h.db.Preload("Category").First(&product, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, http.StatusNotFound, "producto no encontrado")
			return
		}
		respondError(w, http.StatusInternalServerError, "error buscando producto")
		return
	}

	// Se añadió la respuesta exitosa que faltaba
	respondJSON(w, http.StatusOK, product)
}

type UpdateProductRequest struct {
	Name       *string  `json:"name"`
	Price      *float64 `json:"price"`
	Cost       *float64 `json:"cost"`
	CategoryID *uint    `json:"category_id"`
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var req UpdateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	var product models.Product
	if err := h.db.First(&product, id).Error; err != nil {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		if *req.Name == "" {
			respondError(w, http.StatusBadRequest, "name vacío")
			return
		}
		updates["name"] = *req.Name
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
		updates["category_id"] = *req.CategoryID
	}

	if len(updates) == 0 {
		respondError(w, http.StatusBadRequest, "nada que actualizar")
		return
	}

	// Se removió el json.NewEncoder huérfano que causaba el error undefined

	h.db.Model(&product).Updates(updates)

	h.db.Preload("Category").First(&product, product.ID)
	respondJSON(w, http.StatusOK, product)
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var product models.Product

	if err := h.db.First(&product, id).Error; err != nil {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}

	if !product.Active {
		respondError(w, http.StatusConflict, "ya está eliminado")
		return
	}

	product.Active = false
	h.db.Save(&product)

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "producto eliminado",
	})
}

type AdjustStockRequest struct {
	Tipo   string `json:"tipo"`
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

func (h *ProductHandler) AdjustStock(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
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

	// Se removió el json.NewEncoder huérfano que causaba el error undefined

	var product models.Product

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&product, id).Error; err != nil {
			return err
		}

		before := product.Stock
		after := before

		switch req.Tipo {
		case "set":
			after = req.Amount
		case "add":
			after = before + req.Amount
		case "sub":
			if before < req.Amount {
				return fmt.Errorf("stock insuficiente")
			}
			after = before - req.Amount
		default:
			return fmt.Errorf("tipo inválido")
		}

		product.Stock = after

		if err := tx.Save(&product).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.db.Preload("Category").First(&product, product.ID)

	respondJSON(w, http.StatusOK, product)
}
