// Package slug genera la parte legible de las URLs públicas de producto
// (/productos/<slug>). No depende de ningún otro paquete del proyecto.
package slug

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// MaxLen deja margen dentro de varchar(160) para un sufijo "-NN" de desempate.
const MaxLen = 150

var (
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	digits   = regexp.MustCompile(`^[0-9]+$`)
	accents  = strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ñ", "n", "ç", "c",
	)
)

// FoldFrom y FoldTo son las parejas de caracteres que usa translate() en el SQL
// de búsqueda. Tenerlas aquí, junto a Fold, evita que Go y SQL se desincronicen.
const (
	FoldFrom = "áéíóúüñàèìòùâêîôûäëïöç"
	FoldTo   = "aeiouunaeiouaeiouaeioc"
)

// Fold pasa a minúsculas y quita tildes, sin tocar el resto de caracteres.
func Fold(s string) string {
	return accents.Replace(strings.ToLower(s))
}

// Slugify deja solo minúsculas, dígitos y guiones, sin tildes. Puede devolver
// "" si el texto no tiene ningún carácter latino o numérico.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = accents.Replace(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxLen {
		s = strings.Trim(s[:MaxLen], "-") // ya es ASCII: cortar por bytes es seguro
	}
	return s
}

// FromImageURL devuelve el nombre del archivo de la imagen, sin extensión y
// ya convertido a slug ("" si la URL no sirve).
func FromImageURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" {
		return ""
	}
	return Slugify(strings.TrimSuffix(base, path.Ext(base)))
}

// ForProduct elige la base del slug: primero el nombre del archivo de la foto
// (ya curado), y si no hay foto, el nombre del producto. Nunca devuelve "" ni
// un número puro, para no chocar con las rutas por id (/productos/120).
func ForProduct(imageURL *string, name string) string {
	base := ""
	if imageURL != nil {
		base = FromImageURL(*imageURL)
	}
	if base == "" {
		base = Slugify(name)
	}
	if base == "" {
		return "producto"
	}
	if digits.MatchString(base) {
		return "producto-" + base
	}
	return base
}
