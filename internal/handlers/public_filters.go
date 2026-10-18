package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

const (
	filterMaxGroups   = 6  // atributos distintos a la vez
	filterMaxValues   = 10 // valores por atributo
	filterMaxTotal    = 60 // parámetros f en total
	filterMaxNameLen  = 100
	filterMaxValueLen = 200

	facetMinDistinct  = 2   // un atributo con un solo valor no filtra nada
	facetMaxDistinct  = 40  // más de esto es texto libre (autor, páginas...)
	facetMinCoverage  = 0.5 // debe existir en al menos la mitad de los productos
	facetMaxAttribute = 8
)

// filterSpec: nombre de atributo -> valores (O dentro del mismo atributo, Y entre
// atributos distintos), más rango de precio.
type filterSpec struct {
	attrs    map[string][]string
	minPrice *float64
	maxPrice *float64
}

func parsePrice(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || !(f >= 0 && f <= 1e7) { // la forma negada también rechaza NaN
		return nil, errors.New("precio inválido")
	}
	return &f, nil
}

func parseFilters(v url.Values) (filterSpec, error) {
	spec := filterSpec{attrs: map[string][]string{}}
	if len(v["f"]) > filterMaxTotal {
		return spec, errors.New("demasiados filtros")
	}
	for _, raw := range v["f"] {
		name, value, ok := strings.Cut(raw, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" || value == "" ||
			len([]rune(name)) > filterMaxNameLen || len([]rune(value)) > filterMaxValueLen {
			return spec, errors.New("f inválido (use f=Atributo:Valor)")
		}
		vals, seen := spec.attrs[name]
		if !seen && len(spec.attrs) >= filterMaxGroups {
			return spec, errors.New("demasiados atributos en el filtro")
		}
		dup := false
		for _, x := range vals {
			if x == value {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		if len(vals) >= filterMaxValues {
			return spec, errors.New("demasiados valores para " + name)
		}
		spec.attrs[name] = append(vals, value)
	}
	var err error
	if spec.minPrice, err = parsePrice(v.Get("min_price")); err != nil {
		return spec, errors.New("min_price inválido")
	}
	if spec.maxPrice, err = parsePrice(v.Get("max_price")); err != nil {
		return spec, errors.New("max_price inválido")
	}
	return spec, nil
}

// applyFilters agrega los filtros a una consulta sobre products. "exclude" omite
// un atributo (para contar sus propios valores sin que se anule a sí mismo).
func applyFilters(q *gorm.DB, spec filterSpec, exclude string) *gorm.DB {
	for name, vals := range spec.attrs {
		if name == exclude {
			continue
		}
		q = q.Where("EXISTS (SELECT 1 FROM product_attributes pf WHERE pf.product_id = products.id AND pf.name = ? AND pf.value IN ?)", name, vals)
	}
	if spec.minPrice != nil {
		q = q.Where("products.price >= ?", *spec.minPrice)
	}
	if spec.maxPrice != nil {
		q = q.Where("products.price <= ?", *spec.maxPrice)
	}
	return q
}

type facetValue struct {
	Value string `gorm:"column:value" json:"value"`
	Count int64  `gorm:"column:n" json:"count"`
}

type facetAttribute struct {
	Name   string       `json:"name"`
	Values []facetValue `json:"values"`
}

type facetPrice struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type publicFilters struct {
	Categories []publicCategory `json:"categories"`
	Attributes []facetAttribute `json:"attributes"`
	Price      facetPrice       `json:"price"`
	Total      int64            `json:"total"`
}

// ── GET /public/{slug}/filtros ───────────────────────────────────────────────

func (h *PublicHandler) Filters(w http.ResponseWriter, r *http.Request) {
	b, ok := h.resolve(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()

	spec, err := parseFilters(query)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var catIDs []uint
	if raw := query.Get("category"); raw != "" {
		if len(raw) > publicMaxSlugLen {
			respondError(w, http.StatusNotFound, "categoría no encontrada")
			return
		}
		var cat models.Category
		err := h.db.Where("business_id = ? AND slug = ?", b.ID, raw).First(&cat).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, "categoría no encontrada")
			return
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "error buscando categoría")
			return
		}
		ids, err := categorySubtreeIDs(h.db, b.ID, cat.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "error buscando categoría")
			return
		}
		catIDs = ids
	}
	tokens := searchTokens(query.Get("q"))
	fail := func() { respondError(w, http.StatusInternalServerError, "error obteniendo filtros") }

	// scoped = productos activos del negocio dentro del contexto actual. Cada
	// faceta omite su propio filtro para no quedar vacía al elegir un valor.
	scoped := func(excludeAttr string, skipCategory bool) *gorm.DB {
		q := h.db.Model(&models.Product{}).
			Where("products.business_id = ? AND products.active = ?", b.ID, true)
		if len(catIDs) > 0 && !skipCategory {
			q = q.Where("products.category_id IN ?", catIDs)
		}
		if len(tokens) > 0 {
			q = applySearch(q, tokens)
		}
		return applyFilters(q, spec, excludeAttr)
	}

	out := publicFilters{Categories: []publicCategory{}, Attributes: []facetAttribute{}}

	var direct []categoryCount
	if err := scoped("", true).
		Select("products.category_id AS category_id, COUNT(products.id) AS n").
		Group("products.category_id").
		Scan(&direct).Error; err != nil {
		fail()
		return
	}
	cats, err := loadCategories(h.db, b.ID)
	if err != nil {
		fail()
		return
	}
	out.Categories = categoryTree(cats, directCounts(direct))
	if err := scoped("", false).Count(&out.Total).Error; err != nil {
		fail()
		return
	}

	// Qué atributos merecen ser filtro: se decide sobre TODO el catálogo activo,
	// no sobre el contexto, para que la lista no cambie al filtrar.
	var catalogTotal int64
	if err := h.db.Model(&models.Product{}).
		Where("business_id = ? AND active = ?", b.ID, true).
		Scopes(inCategories(catIDs, "category_id")).
		Count(&catalogTotal).Error; err != nil {
		fail()
		return
	}
	type attrStat struct {
		Name      string `gorm:"column:name"`
		Valores   int64  `gorm:"column:valores"`
		NProducts int64  `gorm:"column:n_products"`
	}
	var stats []attrStat
	if err := h.db.Table("product_attributes AS pa").
		Select("pa.name AS name, COUNT(DISTINCT pa.value) AS valores, COUNT(DISTINCT pa.product_id) AS n_products").
		Joins("JOIN products AS p ON p.id = pa.product_id AND p.business_id = ? AND p.active = ?", b.ID, true).
		Scopes(inCategories(catIDs, "p.category_id")).
		Group("pa.name").
		Scan(&stats).Error; err != nil {
		fail()
		return
	}
	eligible := make([]attrStat, 0, len(stats))
	for _, st := range stats {
		if st.Valores >= facetMinDistinct && st.Valores <= facetMaxDistinct &&
			float64(st.NProducts) >= facetMinCoverage*float64(catalogTotal) {
			eligible = append(eligible, st)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].NProducts != eligible[j].NProducts {
			return eligible[i].NProducts > eligible[j].NProducts
		}
		return eligible[i].Name < eligible[j].Name
	})
	if len(eligible) > facetMaxAttribute {
		eligible = eligible[:facetMaxAttribute]
	}

	for _, st := range eligible {
		var rows []facetValue
		if err := scoped(st.Name, false).
			Joins("JOIN product_attributes fa ON fa.product_id = products.id AND fa.name = ?", st.Name).
			Select("fa.value AS value, COUNT(DISTINCT products.id) AS n").
			Group("fa.value").
			Order("n DESC, fa.value").
			Scan(&rows).Error; err != nil {
			fail()
			return
		}
		if len(rows) > 0 {
			out.Attributes = append(out.Attributes, facetAttribute{Name: st.Name, Values: rows})
		}
	}

	var pr struct {
		Min *float64 `gorm:"column:min_price"`
		Max *float64 `gorm:"column:max_price"`
	}
	if err := h.db.Model(&models.Product{}).
		Where("business_id = ? AND active = ?", b.ID, true).
		Scopes(inCategories(catIDs, "category_id")).
		Select("MIN(price) AS min_price, MAX(price) AS max_price").
		Scan(&pr).Error; err != nil {
		fail()
		return
	}
	if pr.Min != nil && pr.Max != nil {
		out.Price = facetPrice{Min: *pr.Min, Max: *pr.Max}
	}

	publicCache(w)
	respondJSON(w, http.StatusOK, out)
}
