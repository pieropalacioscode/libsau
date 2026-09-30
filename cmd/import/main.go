// cmd/import/main.go — importador del Excel oficial de onboarding (Fase 4).
//
// Uso:
//
//	go run cmd/import/main.go --file=catalogo.xlsx --list-sheets
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --dry-run
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --sheet=Alfaguara
//
// Idempotente: correrlo dos veces con el mismo archivo no duplica productos
// ni atributos (upsert por business_id+sku y por product_id+nombre). Si el
// negocio no existe, termina con error claro y no crea nada. --dry-run
// parsea y valida contra la base de datos pero no escribe nada. --list-sheets
// solo lista las hojas del libro y termina — no requiere --business.
//
// Si el archivo tiene más de una hoja y no se pasa --sheet, el programa se
// detiene y muestra los nombres reales en vez de adivinar cuál usar.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/config"
	"github.com/neocode96/libsau/internal/database"
	"github.com/neocode96/libsau/internal/importer"
	"github.com/neocode96/libsau/internal/models"
)

func main() {
	file := flag.String("file", "", "ruta al Excel de catálogo (formato WooCommerce, 97 columnas)")
	businessSlug := flag.String("business", "", "slug del negocio destino, ej. libros-el-estudiante")
	sheet := flag.String("sheet", "", "nombre de la hoja a leer (vacío = detectar automáticamente)")
	dryRun := flag.Bool("dry-run", false, "parsea y valida sin escribir en la base de datos")
	listSheets := flag.Bool("list-sheets", false, "lista las hojas del Excel y termina, sin importar nada")
	flag.Parse()

	if *listSheets {
		if *file == "" {
			fmt.Println("uso: go run cmd/import/main.go --file=catalogo.xlsx --list-sheets")
			os.Exit(1)
		}
		sheets, err := importer.ListSheets(*file)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		fmt.Println("Hojas encontradas:")
		for _, s := range sheets {
			fmt.Printf("  - %q\n", s)
		}
		return
	}

	if *file == "" || *businessSlug == "" {
		fmt.Println("uso: go run cmd/import/main.go --file=catalogo.xlsx --business=slug-del-negocio [--sheet=Nombre] [--dry-run]")
		fmt.Println("     go run cmd/import/main.go --file=catalogo.xlsx --list-sheets")
		os.Exit(1)
	}

	// Si no se especificó hoja y el libro tiene más de una, no adivinamos
	// cuál usar — se detiene y se muestran los nombres reales. Evita cargar
	// silenciosamente la hoja equivocada (p.ej. una hoja de instrucciones).
	if *sheet == "" {
		sheets, err := importer.ListSheets(*file)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		if len(sheets) > 1 {
			fmt.Printf("⚠️  este archivo tiene %d hojas: %s\n", len(sheets), strings.Join(quoteAll(sheets), ", "))
			fmt.Println("   especifica cuál usar, por ejemplo: --sheet=" + sheets[0])
			os.Exit(1)
		}
	}

	// Hay que cargar la config ANTES de conectar, igual que hace
	// cmd/api/main.go — si no, database.Connect() arma el DSN con
	// host/user/dbname vacíos.
	config.Load()
	db, err := database.Connect()
	if err != nil {
		log.Fatal("❌ PostgreSQL:", err)
	}

	var biz models.Business
	if err := db.Where("slug = ?", *businessSlug).First(&biz).Error; err != nil {
		log.Fatalf("❌ el negocio %q no existe — no se creó ningún producto", *businessSlug)
	}

	result, err := importer.ParseExcel(*file, *sheet)
	if err != nil {
		// Encabezados que no calzan, archivo ilegible, hoja vacía: se corta
		// aquí, antes de tocar la base de datos.
		log.Fatalf("❌ %v", err)
	}

	if len(result.RowErrors) > 0 {
		fmt.Printf("⚠️  %d fila(s) con error de datos (no se importan):\n", len(result.RowErrors))
		for _, e := range result.RowErrors {
			fmt.Printf("   - %s\n", e.Error())
		}
	}

	if *dryRun {
		fmt.Println("── modo --dry-run: no se escribe nada en la base de datos ──")
	}

	created, updated, skipped, attrsTotal := 0, 0, 0, 0
	newCategories := map[string]bool{}
	seenSKU := make(map[string]int, len(result.Rows))
	costWarned := false

	for _, row := range result.Rows {
		seenSKU[row.SKU]++
		if seenSKU[row.SKU] > 1 {
			fmt.Printf("⚠️  fila %d: SKU %q repetido dentro del archivo — se queda con los datos de esta fila\n", row.RowIndex, row.SKU)
		}

		catName := lastCategoryLevel(row.Categorias)
		if catName == "" {
			fmt.Printf("⚠️  fila %d: SKU %q sin categoría — se omite\n", row.RowIndex, row.SKU)
			skipped++
			continue
		}

		catID, catIsNew, err := resolveCategory(db, biz.ID, catName, *dryRun)
		if err != nil {
			fmt.Printf("⚠️  fila %d: no se pudo resolver la categoría %q: %v — se omite\n", row.RowIndex, catName, err)
			skipped++
			continue
		}
		if catIsNew {
			newCategories[catName] = true
		}

		// Este formato de WooCommerce no trae columna de costo — se avisa
		// una sola vez, no por cada fila.
		if !costWarned {
			costWarned = true
			fmt.Println("ℹ️  el Excel no trae columna de costo: todos los productos quedan con cost=0 (ajustar luego a mano)")
		}

		productID, wasCreated, err := upsertProduct(db, biz.ID, catID, row, *dryRun)
		if err != nil {
			fmt.Printf("⚠️  fila %d: no se pudo guardar el producto SKU %q: %v\n", row.RowIndex, row.SKU, err)
			skipped++
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}

		attrsWritten, err := upsertAttributes(db, productID, row.Atributos, *dryRun)
		if err != nil {
			fmt.Printf("⚠️  fila %d: SKU %q se guardó, pero falló al guardar sus atributos: %v\n", row.RowIndex, row.SKU, err)
		}
		attrsTotal += attrsWritten
	}

	verb := "creados"
	if *dryRun {
		verb = "se crearían"
	}
	fmt.Printf("\n✅ negocio %q: %d %s, %d actualizados/a actualizar, %d omitidos, %d con error de datos, %d categoría(s) nueva(s), %d atributo(s)\n",
		*businessSlug, created, verb, updated, skipped, len(result.RowErrors), len(newCategories), attrsTotal)
	if *dryRun && len(newCategories) > 0 {
		for name := range newCategories {
			fmt.Printf("   - categoría nueva: %q\n", name)
		}
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// lastCategoryLevel usa solo el último nivel de una jerarquía "A > B > C",
// según fase_04_importador_excel.md.
func lastCategoryLevel(raw string) string {
	parts := strings.Split(raw, ">")
	return strings.TrimSpace(parts[len(parts)-1])
}

// normalizeMaybeEmpty trata "-" (el placeholder que usa este Excel para "sin
// dato") igual que una celda vacía.
func normalizeMaybeEmpty(v string) string {
	v = strings.TrimSpace(v)
	if v == "-" {
		return ""
	}
	return v
}

// isbnFromRow prioriza la columna GTIN/UPC/EAN/ISBN; si viene vacía, cae al
// campo ISBN de los atributos (en la práctica traen el mismo valor). nil si
// ninguna de las dos trae dato real.
func isbnFromRow(row importer.ProductRow) *string {
	v := normalizeMaybeEmpty(row.GTIN)
	if v == "" {
		v = normalizeMaybeEmpty(row.ISBN)
	}
	if v == "" {
		return nil
	}
	return &v
}

// resolveCategory reutiliza la categoría si ya existe para ese negocio, o la
// crea si no. Nunca reutiliza ni crea categorías de otro negocio.
// En --dry-run no escribe: si no existe, devuelve isNew=true sin crearla.
func resolveCategory(db *gorm.DB, businessID uint, name string, dryRun bool) (id uint, isNew bool, err error) {
	var cat models.Category
	err = db.Where("business_id = ? AND name = ?", businessID, name).First(&cat).Error
	if err == nil {
		return cat.ID, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}

	if dryRun {
		return 0, true, nil
	}

	cat = models.Category{BusinessID: businessID, Name: name, Active: true}
	if err := db.Create(&cat).Error; err != nil {
		return 0, false, err
	}
	return cat.ID, true, nil
}

// upsertProduct crea o actualiza por (business_id, sku). Devuelve el ID real
// del producto (0 en --dry-run, donde nunca se escribe). El costo nunca se
// pisa en un update: si ya se cargó a mano una vez, una reimportación no lo
// vuelve a 0.
func upsertProduct(db *gorm.DB, businessID, categoryID uint, row importer.ProductRow, dryRun bool) (id uint, created bool, err error) {
	var existing models.Product
	err = db.Where("business_id = ? AND sku = ?", businessID, row.SKU).First(&existing).Error
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !notFound {
		return 0, false, err
	}

	if dryRun {
		if notFound {
			return 0, true, nil
		}
		return existing.ID, false, nil
	}

	if notFound {
		p := models.Product{
			BusinessID: businessID,
			Name:       row.Nombre,
			SKU:        row.SKU,
			ISBN:       isbnFromRow(row),
			Price:      row.PrecioNormal,
			Cost:       0,
			Stock:      row.Inventario,
			CategoryID: categoryID,
			Active:     true,
		}
		if err := db.Create(&p).Error; err != nil {
			return 0, false, err
		}
		return p.ID, true, nil
	}

	updates := map[string]any{
		"name":        row.Nombre,
		"isbn":        isbnFromRow(row),
		"price":       row.PrecioNormal,
		"stock":       row.Inventario,
		"category_id": categoryID,
	}
	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		return 0, false, err
	}
	return existing.ID, false, nil
}

// skipAttributeNames son los atributos del Excel que ya se guardan en otra
// parte del modelo — guardarlos también aquí sería duplicar el dato.
var skipAttributeNames = map[string]bool{
	"isbn":      true, // ya vive en products.isbn
	"categoría": true, // ya vive en categories, vía la columna Categorías
}

// upsertAttributes guarda los bloques de atributo del Excel (Autor, Editorial,
// Año de edición, Tamaño, etc.) como filas nombre/valor en product_attributes.
// Ignora bloques vacíos, con "-" (el placeholder de "sin dato" de este
// formato), o que ya están cubiertos por otra columna. En --dry-run cuenta
// cuántos se escribirían sin tocar la base de datos.
func upsertAttributes(db *gorm.DB, productID uint, atributos [16]importer.Atributo, dryRun bool) (written int, err error) {
	for _, a := range atributos {
		name := strings.TrimSpace(a.Nombre)
		value := normalizeMaybeEmpty(a.Valor)
		if name == "" || value == "" {
			continue
		}
		if skipAttributeNames[strings.ToLower(name)] {
			continue
		}

		if dryRun {
			written++
			continue
		}

		var existing models.ProductAttribute
		err := db.Where("product_id = ? AND name = ?", productID, name).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := db.Create(&models.ProductAttribute{ProductID: productID, Name: name, Value: value}).Error; err != nil {
				return written, err
			}
		case err != nil:
			return written, err
		case existing.Value != value:
			if err := db.Model(&existing).Update("value", value).Error; err != nil {
				return written, err
			}
		}
		written++
	}
	return written, nil
}
