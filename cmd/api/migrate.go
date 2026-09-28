package main

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/models"
)

func migrate(db *gorm.DB) error {
	// 1. Business primero: las demás tablas dependen de ella.
	if err := db.AutoMigrate(&models.Business{}); err != nil {
		return fmt.Errorf("business: %w", err)
	}

	// 2. Seed idempotente del negocio por defecto.
	def := models.Business{
		Slug: models.DefaultBusinessSlug, Name: "Librería Saber",
		Tier: models.TierFull, Active: true,
	}
	if err := db.Where("slug = ?", def.Slug).FirstOrCreate(&def).Error; err != nil {
		return fmt.Errorf("seed business: %w", err)
	}

	// 3. Tablas preexistentes: business_id con backfill antes de NOT NULL.
	for _, t := range []struct {
		table string
		model any
	}{
		{"categories", &models.Category{}},
		{"products", &models.Product{}},
		{"sales", &models.Sale{}},
	} {
		if err := addTenantColumn(db, t.table, t.model, def.ID); err != nil {
			return err
		}
	}

	// 4. El resto del esquema (FKs, índices).
	//    StockAdjustmentLog estaba en la migrate() vieja de main.go: se conserva.
	if err := db.AutoMigrate(
		&models.Category{}, &models.Product{}, &models.User{},
		&models.Sale{}, &models.SaleItem{}, &models.CashClose{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	// 4b. AutoMigrate no quita NOT NULL de columnas existentes: se hace a mano.
	//     Los pedidos WOO nacen sin usuario hasta que alguien los confirma.
	//     (Idempotente: si ya es nullable, Postgres no da error.)
	if err := db.Exec(`ALTER TABLE sales ALTER COLUMN user_id DROP NOT NULL`).Error; err != nil {
		return fmt.Errorf("sales.user_id nullable: %w", err)
	}

	// 5. El SKU deja de ser único global: ahora lo es por negocio.
	//    Si el Paso 1 muestra otro nombre, cámbialo aquí.
	if err := db.Exec(`ALTER TABLE products DROP CONSTRAINT IF EXISTS uni_products_sku`).Error; err != nil {
		return fmt.Errorf("drop unique sku: %w", err)
	}
	return nil
}

func addTenantColumn(db *gorm.DB, table string, model any, businessID uint) error {
	m := db.Migrator()
	if !m.HasTable(table) || m.HasColumn(model, "business_id") {
		return nil // tabla nueva (AutoMigrate la crea completa) o ya migrada
	}
	log.Printf("↻ %s: agregando business_id (backfill → negocio %d)", table, businessID)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN business_id bigint", table)).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("UPDATE %s SET business_id = ?", table), businessID).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN business_id SET NOT NULL", table)).Error
	})
}
