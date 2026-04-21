package models

type Category struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type Product struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	Name       string   `json:"name"`
	SKU        string   `gorm:"unique" json:"sku"`
	Price      float64  `json:"price"`
	Stock      int      `json:"stock"`
	CategoryID uint     `json:"category_id"`
	Category   Category `gorm:"foreignKey:CategoryID" json:"category"`
	Cost       float64  `gorm:"type:numeric(10,2);not null;default:0" json:"cost"`
	Active     bool     `gorm:"default:true" json:"active"`
}
