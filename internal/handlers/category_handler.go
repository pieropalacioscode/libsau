package handlers

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/models"
)

type CategoryHandler struct {
	db *gorm.DB
}

func NewCategoryHandler(db *gorm.DB) *CategoryHandler {
	return &CategoryHandler{db: db}
}

// GET /api/v1/categories — lista solo las categorías activas.
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	var categories []models.Category
	if err := h.db.Where("active = ?", true).Find(&categories).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error listando categorías")
		return
	}
	respondJSON(w, http.StatusOK, categories)
}

// POST /api/v1/categories — crea una categoría nueva.
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name es requerido")
		return
	}

	cat := models.Category{Name: req.Name, Active: true}
	if err := h.db.Create(&cat).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error creando categoría")
		return
	}
	respondJSON(w, http.StatusCreated, cat)
}
