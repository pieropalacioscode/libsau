package main

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

// maxImageRefLen coincide con varchar(500) de products.image_url.
const maxImageRefLen = 500

var imagePrefixRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(/[a-z0-9][a-z0-9_-]*)*$`)

// cleanImagePrefix valida la carpeta del negocio dentro del bucket ("" = sin prefijo).
func cleanImagePrefix(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "", nil
	}
	if !imagePrefixRe.MatchString(p) {
		return "", fmt.Errorf("%q no es válido: usa minúsculas, dígitos, - y _ (ej. libprep)", p)
	}
	return p, nil
}

// imageRefFromRow devuelve lo que se guarda en products.image_url: nil si el
// Excel no trae foto. Con prefijo guarda la ruta relativa "<prefijo>/<archivo>"
// (la URL pública la arma MEDIA_BASE_URL). Sin prefijo exige una URL https
// completa, como antes.
func imageRefFromRow(row importer.ProductRow, prefix string) (*string, error) {
	raw := normalizeMaybeEmpty(row.Imagenes)
	if raw == "" {
		return nil, nil
	}
	raw = strings.TrimSpace(strings.Split(raw, ",")[0]) // WooCommerce: la primera es la principal

	if prefix == "" {
		if !strings.HasPrefix(raw, "https://") || len(raw) > maxImageRefLen {
			return nil, fmt.Errorf("URL de imagen inválida o de más de %d caracteres: %q", maxImageRefLen, raw)
		}
		return &raw, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("URL de imagen ilegible: %q", raw)
	}
	esc := u.EscapedPath() // conserva %20 y similares; descarta ?query y #fragmento
	file := path.Base(esc)
	if esc == "" || strings.HasSuffix(esc, "/") || file == "." || file == "/" {
		return nil, fmt.Errorf("la imagen no trae nombre de archivo: %q", raw)
	}
	ref := prefix + "/" + file
	if len(ref) > maxImageRefLen {
		return nil, fmt.Errorf("ruta de imagen de más de %d caracteres: %q", maxImageRefLen, ref)
	}
	return &ref, nil
}
