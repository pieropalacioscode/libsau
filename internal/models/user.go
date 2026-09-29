package models

import (
	"time"

	"gorm.io/gorm"
)

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleSeller    Role = "vendedor"
	RoleWarehouse Role = "almacen"
)

type User struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"not null" json:"name"`
	Email        string `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string `gorm:"not null" json:"-"`
	Role         Role   `gorm:"default:'vendedor'" json:"role"`
	Active       bool   `gorm:"default:true" json:"active"`
	// nil = admin de plataforma (todos los negocios); con valor = atado a ese negocio
	BusinessID *uint `gorm:"index" json:"business_id,omitempty"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
