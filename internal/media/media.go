// Package media convierte lo que la BD guarda de una imagen en su URL pública.
//
// La BD guarda una ruta relativa al almacenamiento ("libprep/portada.avif"); el
// dominio donde viven las fotos es solo configuración (MEDIA_BASE_URL). Por
// compatibilidad también se aceptan URLs completas (http/https), que se
// devuelven sin tocar: sirven para fotos alojadas en otro sitio.
package media

import (
	"path"
	"strings"
)

// Resolver arma URLs públicas a partir de la base configurada.
type Resolver struct{ base string }

// NewResolver recibe la base, ej. "https://img.kelbazar.com" (la barra final sobra).
func NewResolver(base string) Resolver {
	return Resolver{base: strings.TrimRight(strings.TrimSpace(base), "/")}
}

// URL devuelve la URL pública de una imagen, o "" si no se puede armar
// (clave vacía, o ruta relativa sin base configurada).
func (r Resolver) URL(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if isAbsolute(key) {
		return key
	}
	if r.base == "" {
		return ""
	}
	// path.Clean impide que un ".." salga de la base.
	return r.base + path.Clean("/"+key)
}

// Ptr es URL para campos opcionales: nil si no hay imagen utilizable.
func (r Resolver) Ptr(key *string) *string {
	if key == nil {
		return nil
	}
	u := r.URL(*key)
	if u == "" {
		return nil
	}
	return &u
}

func isAbsolute(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://")
}
