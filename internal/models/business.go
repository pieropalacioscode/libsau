// internal/models/business.go
package models

import "time"

// Business — un tenant de la plataforma. Librería Saber es uno; cada cliente
// nuevo de catálogo (mini-tienda) es otro.
type Business struct {
	ID             uint    `gorm:"primaryKey" json:"id"`
	Slug           string  `gorm:"uniqueIndex;not null;type:varchar(50)" json:"slug"`
	Name           string  `gorm:"not null;type:varchar(150)" json:"name"`
	Tier           string  `gorm:"type:varchar(20);not null;default:'CATALOG';check:tier IN ('FULL','CATALOG')" json:"tier"`
	WhatsappNumber *string `gorm:"type:varchar(20)" json:"whatsapp_number,omitempty"`
	LogoURL        *string `gorm:"type:varchar(500)" json:"logo_url,omitempty"`
	PrimaryColor   *string `gorm:"type:varchar(7)" json:"primary_color,omitempty"`

	// Marca y datos de contacto opcionales: nulos hasta que el cliente los confirme.
	AccentColor  *string `gorm:"type:varchar(7)" json:"accent_color,omitempty"`
	InstagramURL *string `gorm:"type:varchar(300)" json:"instagram_url,omitempty"`
	TikTokURL    *string `gorm:"column:tiktok_url;type:varchar(300)" json:"tiktok_url,omitempty"`
	FacebookURL  *string `gorm:"type:varchar(300)" json:"facebook_url,omitempty"`
	Address      *string `gorm:"type:varchar(300)" json:"address,omitempty"`
	OpeningHours *string `gorm:"type:varchar(300)" json:"opening_hours,omitempty"`

	Active    bool      `gorm:"default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func (Business) TableName() string { return "businesses" }
