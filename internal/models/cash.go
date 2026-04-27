package models

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// CashClose — Cierre de caja diario. Inmutable una vez guardado.
// La firma SHA-256 detecta cualquier intento de modificación posterior.
type CashClose struct {
	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`

	// Fecha del cierre (un solo cierre por día)
	Date string `gorm:"type:date;uniqueIndex;not null" json:"date"`

	// Quién cerró
	UserID   uint   `gorm:"not null" json:"user_id"`
	UserName string `gorm:"type:varchar(200)" json:"user_name"`

	// Lo que el sistema calculó automáticamente
	SalesCount  int64   `gorm:"not null;default:0" json:"sales_count"`
	RevenueCalc float64 `gorm:"type:numeric(10,2);not null;default:0" json:"revenue_calc"`
	CashCalc    float64 `gorm:"type:numeric(10,2);not null;default:0" json:"cash_calc"`
	YapeCalc    float64 `gorm:"type:numeric(10,2);not null;default:0" json:"yape_calc"`
	PlinCalc    float64 `gorm:"type:numeric(10,2);not null;default:0" json:"plin_calc"`
	TarjetaCalc float64 `gorm:"type:numeric(10,2);not null;default:0" json:"tarjeta_calc"`

	// Lo que el vendedor declaró tener físicamente
	CashDeclared float64 `gorm:"type:numeric(10,2);not null;default:0" json:"cash_declared"`

	// Diferencia: declarado - calculado (negativo = faltante, positivo = sobrante)
	Difference float64 `gorm:"type:numeric(10,2);not null;default:0" json:"difference"`

	Notes string `gorm:"type:varchar(500)" json:"notes"`

	// Firma SHA-256 del registro — detecta tampering
	Hash string `gorm:"type:varchar(64);not null" json:"hash"`

	ClosedAt time.Time `gorm:"not null" json:"closed_at"`
}

func (CashClose) TableName() string { return "cash_closes" }

// ComputeHash genera la firma del cierre para detección de tampering.
// Si alguien modifica un campo en BD, el hash ya no coincidirá.
func (c *CashClose) ComputeHash() string {
	date := c.Date
	if len(date) > 10 {
		date = date[:10] // ← asegura YYYY-MM-DD
	}

	raw := fmt.Sprintf(
		"%s|%d|%d|%.2f|%.2f|%.2f|%.2f|%.2f|%.2f",
		date,
		c.UserID,
		c.SalesCount,
		c.RevenueCalc,
		c.CashCalc,
		c.YapeCalc,
		c.PlinCalc,
		c.TarjetaCalc,
		c.CashDeclared,
	)

	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}
