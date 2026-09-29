package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/middleware"
	"github.com/neocode96/libsau/internal/models"
)

type CategoryHandler struct{ db *gorm.DB }

func NewCategoryHandler(db *gorm.DB) *CategoryHandler { return &CategoryHandler{db: db} }

// GET /api/v1/categories — solo las del negocio resuelto.
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	var categories []models.Category
	if err := h.db.
		Where("business_id = ? AND active = ?", middleware.BusinessID(r), true).
		Order("name ASC").
		Find(&categories).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error listando categorías")
		return
	}
	respondJSON(w, http.StatusOK, categories)
}

// POST /api/v1/categories
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		respondError(w, http.StatusBadRequest, "name es requerido")
		return
	}

	cat := models.Category{BusinessID: middleware.BusinessID(r), Name: name, Active: true}
	if err := h.db.Create(&cat).Error; err != nil {
		if isDuplicateKey(err) {
			respondError(w, http.StatusConflict, "ya existe esa categoría en este negocio")
			return
		}
		respondError(w, http.StatusInternalServerError, "error creando categoría")
		return
	}
	respondJSON(w, http.StatusCreated, cat)
}
