package handlers

import (
	"net/http"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

type categoryRow struct {
	ID       uint   `gorm:"column:id"`
	Name     string `gorm:"column:name"`
	Slug     string `gorm:"column:slug"`
	ParentID *uint  `gorm:"column:parent_id"`
}

type categoryCount struct {
	CategoryID uint  `gorm:"column:category_id"`
	N          int64 `gorm:"column:n"`
}

func loadCategories(db *gorm.DB, businessID uint) ([]categoryRow, error) {
	var rows []categoryRow
	err := db.Table("categories").
		Select("id, name, coalesce(slug, '') AS slug, parent_id").
		Where("business_id = ?", businessID).
		Order("name").
		Scan(&rows).Error
	return rows, err
}

// subtreeFrom devuelve la categoría raíz y todas sus descendientes (con tope
// por si algún dato dejara un ciclo).
func subtreeFrom(cats []categoryRow, root uint) []uint {
	children := map[uint][]uint{}
	for _, c := range cats {
		if c.ParentID != nil {
			children[*c.ParentID] = append(children[*c.ParentID], c.ID)
		}
	}
	out := []uint{root}
	seen := map[uint]bool{root: true}
	for i := 0; i < len(out) && len(out) < 1000; i++ {
		for _, ch := range children[out[i]] {
			if !seen[ch] {
				seen[ch] = true
				out = append(out, ch)
			}
		}
	}
	return out
}

func categorySubtreeIDs(db *gorm.DB, businessID, root uint) ([]uint, error) {
	cats, err := loadCategories(db, businessID)
	if err != nil {
		return nil, err
	}
	return subtreeFrom(cats, root), nil
}

// inCategories restringe una consulta a un conjunto de categorías (no hace
// nada si el conjunto está vacío). col es siempre una constante del código.
func inCategories(ids []uint, col string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(ids) == 0 {
			return db
		}
		return db.Where(col+" IN ?", ids)
	}
}

func directCounts(rows []categoryCount) map[uint]int64 {
	m := make(map[uint]int64, len(rows))
	for _, r := range rows {
		m[r.CategoryID] += r.N
	}
	return m
}

// categoryTree suma el conteo de cada hoja a sus ancestros y devuelve solo las
// categorías con al menos un producto (los padres aparecen aunque no tengan
// productos propios).
func categoryTree(cats []categoryRow, direct map[uint]int64) []publicCategory {
	byID := make(map[uint]categoryRow, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	total := make(map[uint]int64, len(direct))
	for id, n := range direct {
		total[id] += n
		cur := byID[id]
		for depth := 0; cur.ParentID != nil && depth < 10; depth++ {
			p, ok := byID[*cur.ParentID]
			if !ok {
				break
			}
			total[p.ID] += n
			cur = p
		}
	}
	out := []publicCategory{}
	for _, c := range cats {
		if total[c.ID] == 0 {
			continue
		}
		pc := publicCategory{ID: c.ID, Name: c.Name, Slug: c.Slug, ProductCount: total[c.ID]}
		if c.ParentID != nil {
			if p, ok := byID[*c.ParentID]; ok {
				s := p.Slug
				pc.ParentSlug = &s
			}
		}
		out = append(out, pc)
	}
	return out
}

// parentRef arma la referencia al padre de una categoría (nil si no tiene).
func (h *PublicHandler) parentRef(businessID uint, parentID *uint) *publicCategoryRef {
	if parentID == nil {
		return nil
	}
	var c models.Category
	if err := h.db.Where("business_id = ? AND id = ?", businessID, *parentID).First(&c).Error; err != nil {
		return nil
	}
	return &publicCategoryRef{ID: c.ID, Name: c.Name, Slug: c.Slug}
}

// ── GET /public/{slug}/categorias ────────────────────────────────────────────
// Todas las categorías con productos activos; cada padre cuenta los de sus
// hijas. parent_slug es null en los grupos raíz.

func (h *PublicHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}
	cats, err := loadCategories(h.db, b.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo categorías")
		return
	}
	var direct []categoryCount
	if err := h.db.Table("products").
		Select("category_id, COUNT(id) AS n").
		Where("business_id = ? AND active = ?", b.ID, true).
		Group("category_id").
		Scan(&direct).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error obteniendo categorías")
		return
	}
	publicCache(w)
	respondJSON(w, http.StatusOK, categoryTree(cats, directCounts(direct)))
}
