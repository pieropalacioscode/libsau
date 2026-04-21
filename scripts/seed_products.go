//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Product struct {
	ID         uint
	Name       string
	SKU        string
	Price      float64
	Cost       float64
	Stock      int
	CategoryID uint
	Active     bool
}

func main() {
	godotenv.Load("../.env")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"), os.Getenv("DB_PORT"), os.Getenv("DB_SSLMODE"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	products := []Product{
		{
			Name:       "Cuaderno A4",
			SKU:        "CUA-001",
			Price:      6.0,
			Cost:       4.0,
			Stock:      10,
			CategoryID: 1,
			Active:     true,
		},
		{
			Name:       "Lápiz HB",
			SKU:        "LAP-001",
			Price:      5.0,
			Cost:       3.0,
			Stock:      20,
			CategoryID: 1,
			Active:     true,
		},
		{
			Name:       "Lapicero Azul",
			SKU:        "LAP-002",
			Price:      3.5,
			Cost:       2.0,
			Stock:      15,
			CategoryID: 1,
			Active:     true,
		},
	}

	for _, p := range products {
		db.Where("sku = ?", p.SKU).FirstOrCreate(&p)
	}

	fmt.Println("✅ Productos creados")
}
