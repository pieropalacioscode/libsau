package models

type Category struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	BusinessID uint     `gorm:"not null;index;uniqueIndex:uq_category_business_name" json:"business_id"`
	Business   Business `gorm:"foreignKey:BusinessID" json:"-"`
	Name       string   `gorm:"uniqueIndex:uq_category_business_name;not null" json:"name"`
	Active     bool     `json:"active"`
}

type Product struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	BusinessID uint     `gorm:"not null;index;uniqueIndex:uq_product_business_sku" json:"business_id"`
	Business   Business `gorm:"foreignKey:BusinessID" json:"-"`
	Name       string   `json:"name"`
	SKU        string   `gorm:"uniqueIndex:uq_product_business_sku" json:"sku"`
	Price      float64  `gorm:"type:numeric(10,2);not null;default:0" json:"price"`
	Stock      int      `json:"stock"`
	CategoryID uint     `json:"category_id"`
	Category   Category `gorm:"foreignKey:CategoryID" json:"category"`
	Cost       float64  `gorm:"type:numeric(10,2);not null;default:0" json:"cost"`
	Active     bool     `gorm:"default:true" json:"active"`
}
