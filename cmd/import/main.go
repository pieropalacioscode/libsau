// cmd/import/main.go — importador del Excel oficial de onboarding (Fase 4).
//
// Uso:
//
//	go run cmd/import/main.go --file=catalogo.xlsx --list-sheets
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --dry-run
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --sheet=Alfaguara
//	go run cmd/import/main.go --file=catalogo.xlsx --business=libros-el-estudiante --image-prefix=libprep
//
// Idempotente: correrlo dos veces con el mismo archivo no duplica productos
// ni atributos (upsert por business_id+sku y por product_id+nombre). Si el
// negocio no existe, termina con error claro y no crea nada. --dry-run
// parsea y valida contra la base de datos pero no escribe nada. --list-sheets
// solo lista las hojas del libro y termina — no requiere --business.
//
// Si el archivo tiene más de una hoja y no se pasa --sheet, el programa se
// detiene y muestra los nombres reales en vez de adivinar cuál usar.
//
// Imágenes: se toma la primera URL de la columna de imágenes. Con
// --image-prefix se conserva solo el nombre del archivo y se guarda como ruta
// relativa <prefijo>/<archivo> (la URL pública la arma MEDIA_BASE_URL). Una reimportación nunca borra una foto ya cargada.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/pieropalacioscode/ladmincabo/internal/config"
	"github.com/pieropalacioscode/ladmincabo/internal/database"
	"github.com/pieropalacioscode/ladmincabo/internal/importer"
	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

func main() {
	file := flag.String("file", "", "ruta al Excel de catálogo (formato WooCommerce, 97 columnas)")
	businessSlug := flag.String("business", "", "slug del negocio destino, ej. libros-el-estudiante")
	sheet := flag.String("sheet", "", "nombre de la hoja a leer (vacío = detectar automáticamente)")
	dryRun := flag.Bool("dry-run", false, "parsea y valida sin escribir en la base de datos")
	listSheets := flag.Bool("list-sheets", false, "lista las hojas del Excel y termina, sin importar nada")
	imagePrefix := flag.String("image-prefix", "", "carpeta del negocio en el bucket, ej. libprep: guarda la imagen como ruta relativa <prefijo>/<archivo> (vacío = usar la URL https completa del Excel)")
	respectPublished := flag.Bool("respect-published", false, "respeta la columna Publicado: los que no valen 1 quedan inactivos (por defecto todo queda activo)")
	strictISBN := flag.Bool("strict-isbn", false, "valida el ISBN/GTIN (dígito de control y códigos de ejemplo), convierte ISBN-10 a ISBN-13 y vacía lo inválido; sin esto solo se vacían los repetidos")
	flag.Parse()

	prefix, err := cleanImagePrefix(*imagePrefix)
	if err != nil {
		log.Fatalf("❌ --image-prefix: %v", err)
	}

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
		fmt.Println("uso: go run cmd/import/main.go --file=catalogo.xlsx --business=slug-del-negocio [--sheet=Nombre] [--image-prefix=carpeta] [--dry-run]")
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
	cats := newCategoryResolver(db, biz.ID, *dryRun)
	inactive := 0
	provStats := newProviderStats()
	isbns := newISBNGuard(*strictISBN, func(isbn, sku string) string {
		var skus []string
		if err := db.Model(&models.Product{}).Where("business_id = ? AND isbn = ? AND sku <> ?", biz.ID, isbn, sku).Limit(1).Pluck("sku", &skus).Error; err != nil || len(skus) == 0 {
			return ""
		}
		return skus[0]
	})
	seenSKU := make(map[string]int, len(result.Rows))
	costWarned := false

	for _, row := range result.Rows {
		seenSKU[row.SKU]++
		if seenSKU[row.SKU] > 1 {
			fmt.Printf("⚠️  fila %d: SKU %q repetido dentro del archivo — se queda con los datos de esta fila\n", row.RowIndex, row.SKU)
		}

		path := splitCategoryPath(row.Categorias)
		if len(path) == 0 {
			fmt.Printf("⚠️  fila %d: SKU %q sin categoría — se omite\n", row.RowIndex, row.SKU)
			skipped++
			continue
		}

		catID, err := cats.Resolve(path)
		if err != nil {
			fmt.Printf("⚠️  fila %d: SKU %q: categoría %q: %v — se omite\n", row.RowIndex, row.SKU, strings.Join(path, " > "), err)
			skipped++
			continue
		}

		// Este formato de WooCommerce no trae columna de costo — se avisa
		// una sola vez, no por cada fila.
		if !costWarned {
			costWarned = true
			fmt.Println("ℹ️  el Excel no trae columna de costo: todos los productos quedan con cost=0 (ajustar luego a mano)")
		}

		imageURL, imgErr := imageRefFromRow(row, prefix)
		if imgErr != nil {
			fmt.Printf("⚠️  fila %d: SKU %q: imagen ignorada: %v\n", row.RowIndex, row.SKU, imgErr)
		}

		// Un ISBN ya usado por otro SKU (en este archivo o en la base) se importa vacío con un aviso,
		// en vez de omitir el producto.
		isbns.apply(&row)

		if *respectPublished && !row.Publicado {
			inactive++
		}
		productID, wasCreated, err := upsertProduct(db, biz.ID, catID, row, imageURL, *dryRun, *respectPublished)
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

		provider, provErr := providerFromRow(row.Proveedor)
		if provErr != nil {
			fmt.Printf("⚠️  fila %d: SKU %q: proveedor ignorado: %v\n", row.RowIndex, row.SKU, provErr)
		}
		provStats.add(provider)
		if n, err := upsertProvider(db, productID, provider, *dryRun); err != nil {
			fmt.Printf("⚠️  fila %d: SKU %q se guardó, pero falló al guardar su proveedor: %v\n", row.RowIndex, row.SKU, err)
		} else {
			attrsTotal += n
		}
	}

	provStats.report()
	isbns.report()

	verb := "creados"
	if *dryRun {
		verb = "se crearían"
	}
	fmt.Printf("\n✅ negocio %q: %d %s, %d actualizados/a actualizar, %d omitidos, %d con error de datos, %d categoría(s) nueva(s), %d atributo(s)\n",
		*businessSlug, created, verb, updated, skipped, len(result.RowErrors), len(cats.created), attrsTotal)
	if len(cats.created) > 0 {
		fmt.Println("Categorías nuevas:")
		for _, name := range cats.created {
			if n := cats.nodes[name]; n.Parent == "" {
				fmt.Printf("   - %q (grupo)\n", name)
			} else {
				fmt.Printf("   - %q  ← bajo %q\n", name, n.Parent)
			}
		}
	}
	if *respectPublished {
		fmt.Printf("ℹ️  --respect-published: %d producto(s) con Publicado distinto de 1 quedan inactivos\n", inactive)
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
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

// nonEmptyPtr devuelve nil si el valor está vacío (o es "-"); si no, un
// puntero al valor normalizado.
func nonEmptyPtr(s string) *string {
	s = normalizeMaybeEmpty(s)
	if s == "" {
		return nil
	}
	return &s
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

// upsertProduct crea o actualiza por (business_id, sku). Devuelve el ID real
// del producto (0 en --dry-run, donde nunca se escribe). El costo nunca se
// pisa en un update: si ya se cargó a mano una vez, una reimportación no lo
// vuelve a 0. La foto y la descripción solo se escriben si el Excel las trae,
// así una reimportación nunca borra una foto ya cargada.
func upsertProduct(db *gorm.DB, businessID, categoryID uint, row importer.ProductRow, imageURL *string, dryRun, respectPublished bool) (id uint, created bool, err error) {
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

	description := nonEmptyPtr(row.DescripcionCorta)
	active := !respectPublished || row.Publicado

	if notFound {
		p := models.Product{
			BusinessID:  businessID,
			Name:        row.Nombre,
			SKU:         row.SKU,
			ISBN:        isbnFromRow(row),
			ImageURL:    imageURL,
			Description: description,
			Price:       row.PrecioNormal,
			Cost:        0,
			Stock:       row.Inventario,
			CategoryID:  categoryID,
			Active:      active,
		}
		if err := createProduct(db, &p); err != nil {
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
	if imageURL != nil {
		updates["image_url"] = *imageURL
	}
	if description != nil {
		updates["description"] = *description
	}
	if respectPublished {
		updates["active"] = active
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
