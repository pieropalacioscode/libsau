package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

// isbnMaxLen es el tamaño de la columna products.isbn (varchar(20)).
const isbnMaxLen = 20

// isbnPlaceholders son códigos de ejemplo de manuales y plantillas. Tienen dígito de control
// válido, así que validarlo no basta para descartarlos.
var isbnPlaceholders = map[string]bool{
	"5901234123457": true,
	"1234567890128": true,
	"9781234567897": true,
	"9780000000002": true,
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// gs1Valid comprueba el dígito de control (módulo 10) de un EAN-8, UPC-A, EAN-13/ISBN-13 o GTIN-14.
func gs1Valid(s string) bool {
	sum := 0
	for i := 0; i < len(s); i++ {
		d := int(s[len(s)-1-i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

// isbn10Valid comprueba un ISBN-10 (9 dígitos y un dígito o 'X' de control, módulo 11).
func isbn10Valid(s string) bool {
	if len(s) != 10 {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		c := s[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c == 'X' && i == 9:
			v = 10
		default:
			return false
		}
		sum += (10 - i) * v
	}
	return sum%11 == 0
}

// isbn10To13 convierte un ISBN-10 válido en ISBN-13 (prefijo 978 y nuevo dígito de control).
func isbn10To13(s string) string {
	body := "978" + s[:9]
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(body[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return body + string(rune('0'+(10-sum%10)%10))
}

func allSameDigit(s string) bool {
	return s != "" && strings.Count(s, s[:1]) == len(s)
}

// cleanISBN limpia y valida un ISBN/GTIN. Devuelve el valor a guardar; si viene vacío, nota dice por
// qué se descartó. Un ISBN-10 válido sale convertido a ISBN-13 y nota vale "convertido de ISBN-10".
func cleanISBN(raw string) (value, note string) {
	s := strings.ToUpper(strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '\t', '\u00a0':
			return -1
		}
		return r
	}, raw))
	switch {
	case s == "":
		return "", "vacío"
	case len(s) == 10 && isbn10Valid(s):
		return isbn10To13(s), "convertido de ISBN-10"
	case len(s) == 10:
		return "", "ISBN-10 inválido (dígito de control o caracteres)"
	case !isDigits(s):
		return "", "no es un número de ISBN o EAN"
	}
	switch len(s) {
	case 8, 12, 13, 14:
	default:
		return "", fmt.Sprintf("longitud de %d dígitos no válida", len(s))
	}
	// Un 978/979 de 12 dígitos es un ISBN-13 al que le falta un dígito, no un UPC-A.
	if len(s) == 12 && (strings.HasPrefix(s, "978") || strings.HasPrefix(s, "979")) {
		return "", "parece un ISBN-13 incompleto (12 dígitos)"
	}
	if isbnPlaceholders[s] || allSameDigit(s) {
		return "", "es un código de ejemplo"
	}
	if !gs1Valid(s) {
		return "", "dígito de control inválido"
	}
	return s, ""
}

// isbnGuard decide el ISBN que se guarda de cada fila. Un ISBN ya usado por otro SKU del negocio
// (en este archivo o en la base) no omite el producto: se importa con el ISBN vacío y un aviso.
// Con strict, además valida el ISBN y convierte los ISBN-10 válidos a ISBN-13.
type isbnGuard struct {
	strict    bool
	lookup    func(isbn, sku string) string // SKU de la base que ya tiene ese ISBN ("" = nadie)
	owners    map[string]string             // ISBN -> SKU que lo conserva en este archivo
	invalid   int
	converted int
	repeated  int
}

func newISBNGuard(strict bool, lookup func(isbn, sku string) string) *isbnGuard {
	return &isbnGuard{strict: strict, lookup: lookup, owners: map[string]string{}}
}

// decide devuelve el ISBN a guardar ("" = ninguno) y un aviso si hubo que descartarlo.
func (g *isbnGuard) decide(sku, gtin, isbn string) (value, warn string) {
	raw := normalizeMaybeEmpty(gtin)
	if raw == "" {
		raw = normalizeMaybeEmpty(isbn)
	}
	if raw == "" {
		return "", ""
	}
	v := raw
	if g.strict {
		clean, note := cleanISBN(raw)
		if clean == "" {
			g.invalid++
			return "", fmt.Sprintf("ISBN %q descartado (%s); el producto se importa sin ISBN", raw, note)
		}
		if note != "" {
			g.converted++
		}
		v = clean
	}
	if utf8.RuneCountInString(v) > isbnMaxLen {
		g.invalid++
		return "", fmt.Sprintf("ISBN %q descartado (pasa de %d caracteres); el producto se importa sin ISBN", raw, isbnMaxLen)
	}
	if owner, ok := g.owners[v]; ok && owner != sku {
		g.repeated++
		return "", fmt.Sprintf("ISBN %s repetido: ya lo tiene %s en este archivo; este producto se importa sin ISBN", v, owner)
	}
	if g.lookup != nil {
		if other := g.lookup(v, sku); other != "" {
			g.repeated++
			return "", fmt.Sprintf("ISBN %s repetido: ya lo tiene %s en la base; este producto se importa sin ISBN", v, other)
		}
	}
	g.owners[v] = sku
	return v, ""
}

// apply deja en la fila solo el ISBN resuelto (isbnFromRow lo lee de GTIN) y avisa si lo descartó.
func (g *isbnGuard) apply(row *importer.ProductRow) {
	v, warn := g.decide(row.SKU, row.GTIN, row.ISBN)
	if warn != "" {
		fmt.Printf("⚠️  fila %d: SKU %q: %s\n", row.RowIndex, row.SKU, warn)
	}
	row.GTIN, row.ISBN = v, ""
}

func (g *isbnGuard) report() {
	if !g.strict && g.repeated == 0 && g.invalid == 0 {
		return
	}
	fmt.Printf("ℹ️  ISBN: %d repetidos (importados sin ISBN), %d vaciados por inválidos", g.repeated, g.invalid)
	if g.strict {
		fmt.Printf(", %d convertidos de ISBN-10 a ISBN-13", g.converted)
	}
	fmt.Println()
}
