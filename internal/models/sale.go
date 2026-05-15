package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	SaleStatusPending   = "PENDING"
	SaleStatusConfirmed = "CONFIRMED"
	SaleStatusPreparing = "PREPARING"
	SaleStatusShipped   = "SHIPPED"
	SaleStatusDelivered = "DELIVERED"
	SaleStatusCompleted = "COMPLETED"
	SaleStatusCancelled = "CANCELLED"
	SaleStatusRejected  = "REJECTED"
)

// Sale — Cabecera de venta. LOCAL o WOO. Cliente nullable = venta anónima.
type Sale struct {
	ID         uint           `gorm:"primaryKey;autoIncrement"                    json:"id"`
	UserID     uint           `gorm:"not null;index"                              json:"user_id"`
	Origin     string         `gorm:"type:varchar(10);not null;default:'LOCAL'"   json:"origin"`
	ExternalID *string        `gorm:"uniqueIndex" json:"external_id,omitempty"`
	PayMethod  string         `gorm:"type:varchar(20);not null"                   json:"pay_method"`
	Status     string         `gorm:"type:varchar(25);not null;default:'PENDING'" json:"status"`
	Total      float64        `gorm:"type:numeric(10,2);not null;default:0"       json:"total"`
	Notes      *string        `gorm:"type:varchar(300)"                           json:"notes,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index"                                       json:"-"`

	// Relaciones (solo para Preload, no crean columnas)
	User  User       `gorm:"foreignKey:UserID"  json:"user,omitempty"`
	Items []SaleItem `gorm:"foreignKey:SaleID"  json:"items,omitempty"`
}

func (Sale) TableName() string { return "sales" }

// SaleItem — Línea de detalle. Guarda snapshot de precio y costo en el momento
// de la venta. Si el precio cambia mañana, esta venta no se altera.
type SaleItem struct {
	ID            uint    `gorm:"primaryKey;autoIncrement"                       json:"id"`
	SaleID        uint    `gorm:"not null;index:idx_saleitem_sale"               json:"sale_id"`
	ProductID     uint    `gorm:"not null;index:idx_saleitem_product"            json:"product_id"`
	Quantity      int     `gorm:"not null;check:quantity > 0"                    json:"quantity"`
	PriceSnapshot float64 `gorm:"type:numeric(10,2);not null"                    json:"price_snapshot"` // precio al vender
	CostSnapshot  float64 `gorm:"type:numeric(10,2);not null"                    json:"cost_snapshot"`  // costo al vender
	Discount      float64 `gorm:"type:numeric(10,2);not null;default:0"          json:"discount"`
	Subtotal      float64 `gorm:"type:numeric(10,2);not null"                    json:"subtotal"` // calculado en Go

	// Relaciones
	Product Product `gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"product,omitempty"`
}

func (SaleItem) TableName() string { return "sale_items" }

// Subtotal se calcula siempre en Go, nunca confíes en el frontend
func (si *SaleItem) CalculateSubtotal() {
	si.Subtotal = (si.PriceSnapshot - si.Discount) * float64(si.Quantity)
}
