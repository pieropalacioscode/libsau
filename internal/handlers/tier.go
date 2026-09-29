package handlers

import (
	"net/http"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/middleware"
	"github.com/neocode96/libsau/internal/models"
)

// requireFullTier devuelve el business_id del request solo si ese negocio es nivel FULL.
// Si no lo es (o no existe), ya respondió el error y devuelve ok=false: el handler debe hacer return.
// Debe ser lo primero que corre un handler de caja/dashboard, antes de leer el body.
func requireFullTier(db *gorm.DB, w http.ResponseWriter, r *http.Request) (uint, bool) {
	bid := middleware.BusinessID(r)

	var b models.Business
	if err := db.Select("id", "tier").First(&b, bid).Error; err != nil {
		respondError(w, http.StatusNotFound, "negocio no encontrado")
		return 0, false
	}
	if b.Tier != "FULL" {
		respondError(w, http.StatusForbidden, "esta función solo está disponible para negocios de nivel FULL")
		return 0, false
	}
	return bid, true
}
