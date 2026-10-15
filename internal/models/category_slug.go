package models

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/slug"
)

// BeforeCreate asigna el slug de la categoria (URL publica /categoria/<slug>)
// cuando no trae uno. Corre en TODA creacion (importador, panel, seeds). Si ya
// existe en ese negocio, prueba con -2, -3, ...
func (c *Category) BeforeCreate(tx *gorm.DB) error {
	if c.Slug != "" {
		return nil
	}
	base := slug.ForCategory(c.Name)
	candidate := base
	for i := 2; i <= 999; i++ {
		var n int64
		err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&Category{}).
			Where("business_id = ? AND slug = ?", c.BusinessID, candidate).
			Count(&n).Error
		if err != nil {
			return err
		}
		if n == 0 {
			c.Slug = candidate
			return nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Errorf("no se pudo generar un slug unico para la categoria %q", c.Name)
}
