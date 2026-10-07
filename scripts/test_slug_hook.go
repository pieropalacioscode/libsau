//go:build ignore

// scripts/test_slug_hook.go — prueba el hook BeforeCreate de Product.
// Todo ocurre dentro de una transacción que SIEMPRE se revierte: no deja nada
// en la base de datos.
//
//	go run scripts/test_slug_hook.go
package main

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/config"
	"github.com/neocode96/libsau/internal/database"
	"github.com/neocode96/libsau/internal/models"
)

var errRollback = errors.New("rollback intencional")

func main() {
	config.Load()
	db, err := database.Connect()
	if err != nil {
		log.Fatal("❌ PostgreSQL:", err)
	}

	var biz models.Business
	if err := db.Where("slug = ?", "libreria-saber").First(&biz).Error; err != nil {
		log.Fatal("❌ negocio libreria-saber no encontrado:", err)
	}
	var cat models.Category
	if err := db.Where("business_id = ?", biz.ID).First(&cat).Error; err != nil {
		log.Fatal("❌ el negocio no tiene categorías:", err)
	}

	img := "https://img.docentesmart.com/x/mi-foto-demo.avif"
	casos := []models.Product{
		{Name: "Prueba de slug", SKU: "SLUGTEST-1"},
		{Name: "Prueba de slug", SKU: "SLUGTEST-2"},
		{Name: "Otro nombre", SKU: "SLUGTEST-3", ImageURL: &img},
		{Name: "", SKU: "SLUGTEST-4"},
		{Name: "2024", SKU: "SLUGTEST-5"},
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for i := range casos {
			casos[i].BusinessID = biz.ID
			casos[i].CategoryID = cat.ID
			casos[i].Active = true
			if err := tx.Create(&casos[i]).Error; err != nil {
				return err
			}
			fmt.Printf("nombre=%-18q con_imagen=%-5v -> slug %q\n",
				casos[i].Name, casos[i].ImageURL != nil, casos[i].Slug)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		log.Fatalf("❌ la prueba falló antes de terminar: %v", err)
	}
	fmt.Println("↩️  transacción revertida: no quedó nada en la base")
}
