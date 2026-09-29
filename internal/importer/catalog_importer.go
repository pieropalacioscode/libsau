// Package importer parsea y valida los catálogos Excel de 97 columnas
// (estructura WooCommerce: SKU...Proveedor, con 16 bloques de atributos)
// antes de que cmd/import/main.go los inserte/actualice en la base de datos.
package importer

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// CanonicalHeaders es el orden EXACTO de las 97 columnas que debe tener
// cualquier Excel de catálogo (D'Nieve, Librería Saber, papelería, etc.).
// Si el Excel viene con columnas duplicadas o corridas (como pasa con
// "¿Permitir valoraciones de clientes?.1" y "Precio rebajado.1"), la
// validación de abajo lo detecta ANTES de tocar la base de datos.
var CanonicalHeaders = buildCanonicalHeaders()

func buildCanonicalHeaders() []string {
	headers := []string{
		"SKU", "Tipo", "GTIN, UPC, EAN o ISBN", "Nombre", "Publicado", "¿Está destacado?",
		"Visibilidad en el catálogo", "Descripción corta", "¿Permitir valoraciones de clientes?",
		"Precio rebajado", "Inventario", "Cantidad de bajo inventario", "Peso (g)", "Longitud (cm)",
		"Anchura (cm)", "Altura (cm)", "Ilustraciones", "Precio normal", "Categorías", "Imágenes",
		"Ventas dirigidas", "Ventas cruzadas", "Marcas", "ISBN",
	}
	for i := 1; i <= 16; i++ {
		headers = append(headers,
			fmt.Sprintf("Nombre del atributo %d", i),
			fmt.Sprintf("Valor(es) del atributo %d", i),
			fmt.Sprintf("Atributo visible %d", i),
			fmt.Sprintf("Atributo global %d", i),
		)
	}
	headers = append(headers,
		"Meta: _yoast_wpseo_focuskw", "Meta: _yoast_wpseo_focuskw_text_input",
		"Meta: _yoast_wpseo_title", "Meta: _yoast_wpseo_metadesc",
		"Meta: _yoast_wpseo_focuskeywords", "Meta: _yoast_wpseo_keywordsynonyms",
		"Meta: _yoast_wpseo_opengraph-title", "Meta: _yoast_wpseo_opengraph-description",
		"Proveedor",
	)
	return headers
}

// Atributo representa uno de los 16 bloques de atributo de una fila.
// No se usa todavía en cmd/import/main.go — la tabla de atributos controlados
// sigue diferida (ver LIBSAU_v5_02_modelo_de_datos.md).
type Atributo struct {
	Nombre  string
	Valor   string
	Visible bool
	Global  bool
}

// ProductRow es una fila del Excel ya convertida a tipos de Go.
type ProductRow struct {
	RowIndex int // fila real en el Excel (para reportar errores al usuario)

	SKU                  string
	Tipo                 string
	GTIN                 string
	Nombre               string
	Publicado            bool
	Destacado            bool
	Visibilidad          string
	DescripcionCorta     string
	PermiteValoraciones  bool
	PrecioRebajado       *float64 // nil si viene vacío
	Inventario           int      // = "Stock" en el lenguaje del plan de fases
	BajoInventario       int
	PesoG                float64
	LongitudCm           float64
	AnchuraCm            float64
	AlturaCm             float64
	Ilustraciones        string
	PrecioNormal         float64 // = "Precio regular" en el lenguaje del plan de fases
	Categorias           string  // jerarquía separada por ">"
	Imagenes             string
	VentasDirigidas      string
	VentasCruzadas       string
	Marcas               string
	ISBN                 string
	Atributos            [16]Atributo

	YoastFocusKW          string
	YoastFocusKWTextInput string
	YoastTitle            string
	YoastMetaDesc         string
	YoastFocusKeywords    string
	YoastKeywordSynonyms  string
	YoastOGTitle          string
	YoastOGDescription    string
	Proveedor             string
}

// RowError es un error de parseo de una fila puntual (no detiene el import
// completo; se acumula para mostrarle al admin qué filas revisar).
type RowError struct {
	RowIndex int
	Field    string
	Message  string
}

func (e RowError) Error() string {
	return fmt.Sprintf("fila %d, columna %q: %s", e.RowIndex, e.Field, e.Message)
}

// ParseResult agrupa las filas parseadas correctamente y los errores por fila.
type ParseResult struct {
	Rows      []ProductRow
	RowErrors []RowError
}

// HeaderMismatchError se devuelve cuando el Excel no calza con CanonicalHeaders.
// Trae el detalle exacto (qué sobra, qué falta, qué está corrido) para que el
// admin pueda corregir el Excel sin adivinar.
type HeaderMismatchError struct {
	Got      []string
	Expected []string
	Extra    []string // columnas que están de más (p.ej. duplicadas)
	Missing  []string // columnas esperadas que no aparecen
}

func (e *HeaderMismatchError) Error() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("el Excel tiene %d columnas, se esperaban %d. ", len(e.Got), len(e.Expected)))
	if len(e.Extra) > 0 {
		b.WriteString(fmt.Sprintf("Columnas de más/duplicadas: %s. ", strings.Join(e.Extra, ", ")))
	}
	if len(e.Missing) > 0 {
		b.WriteString(fmt.Sprintf("Columnas faltantes: %s. ", strings.Join(e.Missing, ", ")))
	}
	return b.String()
}

// normalizeHeader recorta bordes y colapsa espacios internos dobles. No toca
// mayúsculas, tildes ni puntuación — esos sí distinguen columnas reales
// (p.ej. "GTIN, UPC, EAN o ISBN" vs "ISBN" son columnas distintas).
func normalizeHeader(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ValidateHeaders compara la fila de encabezados del Excel contra
// CanonicalHeaders, normalizando espacios en ambos lados antes de comparar.
// Devuelve nil si coinciden en cantidad y orden.
func ValidateHeaders(got []string) error {
	expected := CanonicalHeaders
	if len(got) == len(expected) {
		match := true
		for i := range expected {
			if normalizeHeader(got[i]) != expected[i] {
				match = false
				break
			}
		}
		if match {
			return nil
		}
	}

	expectedSet := make(map[string]bool, len(expected))
	for _, h := range expected {
		expectedSet[h] = true
	}
	gotSet := make(map[string]int, len(got)) // cuenta ocurrencias -> detecta duplicados
	for _, h := range got {
		gotSet[normalizeHeader(h)]++
	}

	var extra, missing []string
	for h, count := range gotSet {
		if !expectedSet[h] || count > 1 {
			label := h
			if count > 1 {
				label = fmt.Sprintf("%s (aparece %d veces)", h, count)
			}
			extra = append(extra, label)
		}
	}
	for _, h := range expected {
		if gotSet[h] == 0 {
			missing = append(missing, h)
		}
	}

	return &HeaderMismatchError{Got: got, Expected: expected, Extra: extra, Missing: missing}
}

// ParseExcel abre el archivo, valida los encabezados de la hoja indicada
// (por lo general la única hoja del libro: "Alfaguara", "Papelería Escolar",
// "D'Nieve", etc. — si sheetName es "" se usa la primera hoja del libro) y
// devuelve las filas ya convertidas a ProductRow.
func ParseExcel(path string, sheetName string) (*ParseResult, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el Excel: %w", err)
	}
	defer f.Close()
	return parseWorkbook(f, sheetName)
}

// ParseExcelReader es la variante para un futuro panel de admin: recibe el
// archivo subido por HTTP (multipart.File cumple io.Reader) sin necesidad de
// guardarlo primero a disco. No se usa en la Fase 4 (CLI), queda lista para
// cuando exista un endpoint de carga.
func ParseExcelReader(r io.Reader, sheetName string) (*ParseResult, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el Excel subido: %w", err)
	}
	defer f.Close()
	return parseWorkbook(f, sheetName)
}

func parseWorkbook(f *excelize.File, sheetName string) (*ParseResult, error) {
	if sheetName == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("el archivo no tiene hojas")
		}
		sheetName = sheets[0]
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la hoja %q: %w", sheetName, err)
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("la hoja %q está vacía", sheetName)
	}

	header := rows[0]
	if err := ValidateHeaders(header); err != nil {
		return nil, err // corta aquí: preferimos rechazar el archivo entero
	} // antes que importar datos mal alineados.

	result := &ParseResult{}
	for i := 1; i < len(rows); i++ {
		raw := rows[i]
		rowIndex := i + 1 // +1 porque Excel es 1-indexado y ya saltamos el header

		// Normalizamos el largo de la fila a 97 columnas (excelize recorta
		// las celdas vacías del final de cada fila).
		row := make([]string, len(CanonicalHeaders))
		copy(row, raw)

		sku := strings.TrimSpace(row[0])
		if sku == "" {
			continue // fila totalmente vacía (frecuente al final del libro)
		}

		pr, rowErrs := parseRow(rowIndex, row)
		if len(rowErrs) > 0 {
			// Fila inválida: se reporta el error pero NO se agrega a Rows,
			// para que nunca se intente importar con datos incompletos.
			result.RowErrors = append(result.RowErrors, rowErrs...)
			continue
		}
		result.Rows = append(result.Rows, pr)
	}

	return result, nil
}

func parseRow(rowIndex int, c []string) (ProductRow, []RowError) {
	var errs []RowError

	get := func(idx int) string {
		if idx < len(c) {
			return strings.TrimSpace(c[idx])
		}
		return ""
	}

	pr := ProductRow{
		RowIndex:         rowIndex,
		SKU:              get(0),
		Tipo:             get(1),
		GTIN:             get(2),
		Nombre:           get(3),
		Visibilidad:      get(6),
		DescripcionCorta: get(7),
		Ilustraciones:    get(16),
		Categorias:       get(18),
		Imagenes:         get(19),
		VentasDirigidas:  get(20),
		VentasCruzadas:   get(21),
		Marcas:           get(22),
		ISBN:             get(23),
	}

	pr.Publicado = parseBool01(get(4))
	pr.Destacado = parseBool01(get(5))
	pr.PermiteValoraciones = parseBool01(get(8))

	if v := get(9); v != "" && v != "-" {
		f, err := parseFloatEs(v)
		if err != nil {
			errs = append(errs, RowError{rowIndex, "Precio rebajado", "no es un número válido: " + v})
		} else {
			pr.PrecioRebajado = &f
		}
	}

	pr.Inventario = parseIntSafe(get(10), rowIndex, "Inventario", &errs)
	pr.BajoInventario = parseIntSafe(get(11), rowIndex, "Cantidad de bajo inventario", &errs)
	pr.PesoG = parseFloatSafe(get(12), rowIndex, "Peso (g)", &errs)
	pr.LongitudCm = parseFloatSafe(get(13), rowIndex, "Longitud (cm)", &errs)
	pr.AnchuraCm = parseFloatSafe(get(14), rowIndex, "Anchura (cm)", &errs)
	pr.AlturaCm = parseFloatSafe(get(15), rowIndex, "Altura (cm)", &errs)
	pr.PrecioNormal = parseFloatSafe(get(17), rowIndex, "Precio normal", &errs)

	if pr.Nombre == "" {
		errs = append(errs, RowError{rowIndex, "Nombre", "vacío"})
	}
	if pr.PrecioNormal <= 0 {
		errs = append(errs, RowError{rowIndex, "Precio normal", "debe ser mayor a 0"})
	}

	// 16 bloques de atributo, 4 columnas cada uno, empiezan en el índice 24.
	base := 24
	for i := 0; i < 16; i++ {
		off := base + i*4
		pr.Atributos[i] = Atributo{
			Nombre:  get(off),
			Valor:   get(off + 1),
			Visible: parseBool01(get(off + 2)),
			Global:  parseBool01(get(off + 3)),
		}
	}

	// Los 9 campos finales (Yoast + Proveedor) empiezan en 24 + 16*4 = 88.
	seo := base + 16*4
	pr.YoastFocusKW = get(seo)
	pr.YoastFocusKWTextInput = get(seo + 1)
	pr.YoastTitle = get(seo + 2)
	pr.YoastMetaDesc = get(seo + 3)
	pr.YoastFocusKeywords = get(seo + 4)
	pr.YoastKeywordSynonyms = get(seo + 5)
	pr.YoastOGTitle = get(seo + 6)
	pr.YoastOGDescription = get(seo + 7)
	pr.Proveedor = get(seo + 8)

	return pr, errs
}

func parseBool01(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "sí") || strings.EqualFold(v, "si")
}

// parseFloatEs acepta tanto "1234.50" como "1234,50" (coma decimal, común en
// exports hechos a mano en Excel en español).
func parseFloatEs(v string) (float64, error) {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, ",", ".")
	return strconv.ParseFloat(v, 64)
}

func parseFloatSafe(v string, rowIndex int, field string, errs *[]RowError) float64 {
	if v == "" || v == "-" {
		return 0
	}
	f, err := parseFloatEs(v)
	if err != nil {
		*errs = append(*errs, RowError{rowIndex, field, "no es un número válido: " + v})
		return 0
	}
	return f
}

func parseIntSafe(v string, rowIndex int, field string, errs *[]RowError) int {
	if v == "" || v == "-" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		*errs = append(*errs, RowError{rowIndex, field, "no es un entero válido: " + v})
		return 0
	}
	return n
}