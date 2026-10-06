package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/models"
)

// PublicHandler sirve el catálogo de solo lectura, sin JWT, identificado por
// el slug del negocio. Usa los helpers respondJSON / respondError / pathID
// definidos en product_handler.go (mismo package).
type PublicHandler struct {
	db *gorm.DB
}

func NewPublicHandler(db *gorm.DB) *PublicHandler {
	return &PublicHandler{db: db}
}

const (
	publicDefaultLimit = 100
	publicMaxLimit     = 500
)

// ── DTOs públicos ────────────────────────────────────────────────────────────
// Se definen aparte de models.Product a propósito: así `cost`, `stock`,
// `business_id` y `sku` NUNCA pueden filtrarse por accidente al agregar un
// campo nuevo al modelo.

type publicBusiness struct {
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	WhatsappNumber *string `json:"whatsapp_number"`
	LogoURL        *string `json:"logo_url"`
	PrimaryColor   *string `json:"primary_color"`
}

type publicCategoryRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type publicProduct struct {
	ID       uint              `json:"id"`
	Name     string            `json:"name"`
	Price    float64           `json:"price"`
	ImageURL *string           `json:"image_url"`
	Category publicCategoryRef `json:"category"`
}

type publicAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type publicProductDetail struct {
	publicProduct
	ISBN        *string           `json:"isbn,omitempty"`
	Description *string           `json:"description,omitempty"`
	Attributes  []publicAttribute `json:"attributes"`
}

type publicCategory struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	ProductCount int64  `json:"product_count"`
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// resolve convierte {slug} en el negocio. Slug inexistente o negocio inactivo
// => 404 (nunca 500 ni lista vacía silenciosa).
func (h *PublicHandler) resolve(w http.ResponseWriter, r *http.Request) (*models.Business, bool) {
	slug := chi.URLParam(r, "slug")

	var b models.Business
	err := h.db.Where("slug = ? AND active = ?", slug, true).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(w, http.StatusNotFound, "negocio no encontrado")
		return nil, false
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error buscando negocio")
		return nil, false
	}
	return &b, true
}

func publicCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=60")
}

// publicPositiveQuery lee un query param entero >= 1; vacío => def.
func publicPositiveQuery(r *http.Request, key string, def int) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func toPublicProduct(p models.Product) publicProduct {
	return publicProduct{
		ID:       p.ID,
		Name:     p.Name,
		Price:    p.Price,
		ImageURL: p.ImageURL,
		Category: publicCategoryRef{ID: p.Category.ID, Name: p.Category.Name},
	}
}

// ── GET /public/{slug} ───────────────────────────────────────────────────────
// Datos públicos del negocio: lo que el frontend necesita para logo, color y
// el botón de WhatsApp.

func (h *PublicHandler) Business(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}
	publicCache(w)
	respondJSON(w, http.StatusOK, publicBusiness{
		Slug:           b.Slug,
		Name:           b.Name,
		WhatsappNumber: b.WhatsappNumber,
		LogoURL:        b.LogoURL,
		PrimaryColor:   b.PrimaryColor,
	})
}

// ── GET /public/{slug}/productos ─────────────────────────────────────────────
// Query params opcionales: page (def 1), limit (def 100, máx 500), category_id.
// Responde un array; el total va en el header X-Total-Count.

func (h *PublicHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}

	page, ok := publicPositiveQuery(r, "page", 1)
	if !ok {
		respondError(w, http.StatusBadRequest, "page inválido")
		return
	}
	limit, ok := publicPositiveQuery(r, "limit", publicDefaultLimit)
	if !ok {
		respondError(w, http.StatusBadRequest, "limit inválido")
		return
	}
	if limit > publicMaxLimit {
		limit = publicMaxLimit
	}

	// Session() permite reutilizar q para Count y Find sin que se contaminen.
	q := h.db.Model(&models.Product{}).
		Where("business_id = ? AND active = ?", b.ID, true).
		Session(&gorm.Session{})

	if raw := r.URL.Query().Get("category_id"); raw != "" {
		catID, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || catID == 0 {
			respondError(w, http.StatusBadRequest, "category_id inválido")
			return
		}
		q = q.Where("category_id = ?", catID).Session(&gorm.Session{})
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error contando productos")
		return
	}

	var products []models.Product
	if err := q.Preload("Category").
		Order("id").
		Limit(limit).
		Offset((page - 1) * limit).
		Find(&products).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo productos")
		return
	}

	out := make([]publicProduct, 0, len(products))
	for _, p := range products {
		out = append(out, toPublicProduct(p))
	}

	publicCache(w)
	w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	respondJSON(w, http.StatusOK, out)
}

// ── GET /public/{slug}/productos/{id} ────────────────────────────────────────

func (h *PublicHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}

	id, ok := pathID(r)
	if !ok {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var p models.Product
	err := h.db.Preload("Category").
		Where("id = ? AND business_id = ? AND active = ?", id, b.ID, true).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error buscando producto")
		return
	}

	var attrs []models.ProductAttribute
	if err := h.db.Where("product_id = ?", p.ID).Order("id").Find(&attrs).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo atributos")
		return
	}

	detail := publicProductDetail{
		publicProduct: toPublicProduct(p),
		ISBN:          p.ISBN,
		Description:   p.Description,
		Attributes:    make([]publicAttribute, 0, len(attrs)),
	}
	for _, a := range attrs {
		detail.Attributes = append(detail.Attributes, publicAttribute{Name: a.Name, Value: a.Value})
	}

	publicCache(w)
	respondJSON(w, http.StatusOK, detail)
}

// ── GET /public/{slug}/categorias ────────────────────────────────────────────
// Solo categorías que tienen al menos un producto activo en este negocio.
// Hoy el modelo Category no tiene ParentID, así que la lista es plana.

func (h *PublicHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}

	rows := []publicCategory{}
	if err := h.db.Table("categories AS c").
		Select("c.id AS id, c.name AS name, COUNT(p.id) AS product_count").
		Joins("JOIN products AS p ON p.category_id = c.id AND p.business_id = c.business_id AND p.active = ?", true).
		Where("c.business_id = ?", b.ID).
		Group("c.id, c.name").
		Order("c.name").
		Scan(&rows).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo categorías")
		return
	}

	publicCache(w)
	respondJSON(w, http.StatusOK, rows)
}
