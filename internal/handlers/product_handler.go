package handlers

import (
	"encoding/json"
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

	if err := h.db.Preload("Category").Find(&products).Error; err != nil {
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

	respondJSON(w, http.StatusOK, product)

}
