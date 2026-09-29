// cmd/import/main.go — importador del Excel oficial de onboarding (Fase 4).
//
// Uso:
//
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --dry-run
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --sheet=Alfaguara
//
// Idempotente: correrlo dos veces con el mismo archivo no duplica productos
// (upsert por business_id + sku). Si el negocio no existe, termina con error
// claro y no crea nada. --dry-run parsea y valida contra la base de datos
// pero no escribe nada — sirve para revisar antes de confirmar el import.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/importer"
	"github.com/neocode96/libsau/internal/models"
	// TODO: ajusta este import al paquete/función real que abre tu conexión.
	// Debe exponer algo equivalente a database.Connect() *gorm.DB — el mismo
	// que usa cmd/api/main.go. Pega ese fragmento si el nombre no calza.
	"github.com/neocode96/libsau/internal/database"
)

func main() {
	file := flag.String("file", "", "ruta al Excel de catálogo (formato WooCommerce, 97 columnas)")
	businessSlug := flag.String("business", "", "slug del negocio destino, ej. libros-el-estudiante")
	sheet := flag.String("sheet", "", "nombre de la hoja a leer (vacío = primera hoja del libro)")
	dryRun := flag.Bool("dry-run", false, "parsea y valida sin escribir en la base de datos")
	flag.Parse()

	if *file == "" || *businessSlug == "" {
		fmt.Println("uso: go run cmd/import/main.go --file=catalogo.xlsx --business=slug-del-negocio [--sheet=Nombre] [--dry-run]")
		os.Exit(1)
	}

	db := database.Connect() // TODO: ver nota arriba si el nombre real es distinto

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

	created, updated, skipped := 0, 0, 0
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

		wasCreated, err := upsertProduct(db, biz.ID, catID, row, *dryRun)
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
	}

	verb := "creados"
	if *dryRun {
		verb = "se crearían"
	}
	fmt.Printf("\n✅ negocio %q: %d %s, %d actualizados/a actualizar, %d omitidos, %d con error de datos, %d categoría(s) nueva(s)\n",
		*businessSlug, created, verb, updated, skipped, len(result.RowErrors), len(newCategories))
	if *dryRun && len(newCategories) > 0 {
		for name := range newCategories {
			fmt.Printf("   - categoría nueva: %q\n", name)
		}
	}
}

// lastCategoryLevel usa solo el último nivel de una jerarquía "A > B > C",
// según fase_04_importador_excel.md.
func lastCategoryLevel(raw string) string {
	parts := strings.Split(raw, ">")
	return strings.TrimSpace(parts[len(parts)-1])
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

// upsertProduct crea o actualiza por (business_id, sku). El costo nunca se
// pisa en un update: si ya se cargó a mano una vez, una reimportación no lo
// vuelve a 0. En --dry-run no escribe: solo informa qué habría pasado.
func upsertProduct(db *gorm.DB, businessID, categoryID uint, row importer.ProductRow, dryRun bool) (created bool, err error) {
	var existing models.Product
	err = db.Where("business_id = ? AND sku = ?", businessID, row.SKU).First(&existing).Error
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !notFound {
		return false, err
	}

	if dryRun {
		return notFound, nil
	}

	if notFound {
		p := models.Product{
			BusinessID: businessID,
			Name:       row.Nombre,
			SKU:        row.SKU,
			Price:      row.PrecioNormal,
			Cost:       0,
			Stock:      row.Inventario,
			CategoryID: categoryID,
			Active:     true,
		}
		if err := db.Create(&p).Error; err != nil {
			return false, err
		}
		return true, nil
	}

	updates := map[string]any{
		"name":        row.Nombre,
		"price":       row.PrecioNormal,
		"stock":       row.Inventario,
		"category_id": categoryID,
	}
	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		return false, err
	}
	return false, nil
}