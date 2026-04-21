package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/neocode96/libsau/internal/models"
	"gorm.io/gorm"
)

type CategoryHandler struct {
	db *gorm.DB
}

func NewCategoryHandler(db *gorm.DB) *CategoryHandler {
	return &CategoryHandler{db: db}
}

// GET /categories
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	var categories []models.Category

	if err := h.db.Find(&categories).Error; err != nil {
		http.Error(w, "error", 500)
		return
	}

	json.NewEncoder(w).Encode(categories)
}

// POST /categories
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var category models.Category

	if err := json.NewDecoder(r.Body).Decode(&category); err != nil {
		http.Error(w, "json invalido", 400)
		return
	}

	if category.Name == "" {
		http.Error(w, "name requerido", 400)
		return
	}

	category.Active = true

	if err := h.db.Create(&category).Error; err != nil {
		http.Error(w, "error guardando", 500)
		return
	}

	json.NewEncoder(w).Encode(category)
}
