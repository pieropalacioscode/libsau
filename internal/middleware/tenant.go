// internal/middleware/tenant.go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/models"
)

type tenantKey struct{}

// BusinessID devuelve el negocio resuelto por ResolveBusiness (0 = no resuelto).
func BusinessID(r *http.Request) uint {
	id, _ := r.Context().Value(tenantKey{}).(uint)
	return id
}

func ResolveBusiness(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r)
			if claims == nil {
				tenantError(w, http.StatusUnauthorized, "no autenticado")
				return
			}

			var user models.User
			if err := db.Select("id", "business_id").First(&user, claims.UserID).Error; err != nil {
				tenantError(w, http.StatusUnauthorized, "usuario no encontrado")
				return
			}

			requested := requestedBusiness(r)
			var bid uint
			switch {
			case user.BusinessID != nil: // usuario atado a un negocio
				bid = *user.BusinessID
				if requested != 0 && requested != bid {
					tenantError(w, http.StatusForbidden, "sin acceso a ese negocio")
					return
				}
			case requested != 0: // admin de plataforma eligió negocio
				bid = requested
			default: // transitorio hasta el selector de la Fase 2
				var def models.Business
				if err := db.Select("id").Where("slug = ?", models.DefaultBusinessSlug).First(&def).Error; err != nil {
					tenantError(w, http.StatusInternalServerError, "negocio por defecto no configurado")
					return
				}
				bid = def.ID
			}

			var n int64
			db.Model(&models.Business{}).Where("id = ? AND active = ?", bid, true).Count(&n)
			if n == 0 {
				tenantError(w, http.StatusNotFound, "negocio no existe o está inactivo")
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey{}, bid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestedBusiness(r *http.Request) uint {
	v := r.Header.Get("X-Business-ID")
	if v == "" {
		v = r.URL.Query().Get("business_id")
	}
	id, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
}

func tenantError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
