package main

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

// maxCategoryDepth: grupo > hoja. El frontend está diseñado para 2 niveles.
const maxCategoryDepth = 2

// splitCategoryPath parte "Grupo > Hoja" en niveles limpios, sin vacíos.
// Las comas dentro de un nombre NO separan nada.
func splitCategoryPath(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ">") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sameParent(a, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func parentLabel(name string) string {
	if name == "" {
		return "la raíz"
	}
	return fmt.Sprintf("%q", name)
}

type catNode struct {
	ID     uint   // 0 en --dry-run para las nuevas
	Parent string // nombre del padre ("" = raíz)
}

// categoryResolver crea o reutiliza la cadena de categorías de un negocio.
// El nombre es único por negocio, así que se indexa por nombre.
type categoryResolver struct {
	db         *gorm.DB
	businessID uint
	dryRun     bool
	nodes      map[string]*catNode
	created    []string // nombres nuevos, en orden de creación (padres primero)
}

func newCategoryResolver(db *gorm.DB, businessID uint, dryRun bool) *categoryResolver {
	return &categoryResolver{db: db, businessID: businessID, dryRun: dryRun, nodes: map[string]*catNode{}}
}

// Resolve devuelve el ID de la hoja (0 en --dry-run si es nueva).
func (r *categoryResolver) Resolve(path []string) (uint, error) {
	if len(path) == 0 {
		return 0, errors.New("categoría vacía")
	}
	if len(path) > maxCategoryDepth {
		return 0, fmt.Errorf("más de %d niveles", maxCategoryDepth)
	}
	var parentID *uint
	parentName := ""
	var id uint
	for _, name := range path {
		n, err := r.ensure(name, parentName, parentID)
		if err != nil {
			return 0, err
		}
		id = n.ID
		pid := n.ID
		parentID = &pid
		parentName = name
	}
	return id, nil
}

func (r *categoryResolver) ensure(name, parentName string, parentID *uint) (*catNode, error) {
	if n, ok := r.nodes[name]; ok {
		if n.Parent != parentName {
			return nil, fmt.Errorf("%q aparece bajo %s y también bajo %s (el nombre debe ser único por negocio)",
				name, parentLabel(n.Parent), parentLabel(parentName))
		}
		return n, nil
	}

	var cat models.Category
	err := r.db.Where("business_id = ? AND name = ?", r.businessID, name).First(&cat).Error
	switch {
	case err == nil:
		if !sameParent(cat.ParentID, parentID) {
			return nil, fmt.Errorf("%q ya existe en la base con otro padre (el nombre debe ser único por negocio)", name)
		}
		n := &catNode{ID: cat.ID, Parent: parentName}
		r.nodes[name] = n
		return n, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		n := &catNode{Parent: parentName}
		if !r.dryRun {
			cat = models.Category{BusinessID: r.businessID, Name: name, ParentID: parentID, Active: true}
			if err := r.db.Create(&cat).Error; err != nil {
				return nil, err
			}
			n.ID = cat.ID
		}
		r.nodes[name] = n
		r.created = append(r.created, name)
		return n, nil
	default:
		return nil, err
	}
}

// createProduct inserta respetando Active=false: el tag default:true de GORM
// convertiría un false en true al insertar.
func createProduct(db *gorm.DB, p *models.Product) error {
	active := p.Active
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		if !active {
			if err := tx.Model(&models.Product{}).Where("id = ?", p.ID).Update("active", false).Error; err != nil {
				return err
			}
			p.Active = false
		}
		return nil
	})
}
