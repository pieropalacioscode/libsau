package auth

import (
	"context"
	"net/http"
	"os"
	"strings"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// 🧠 MODO DEV: saltarse autenticación
		if os.Getenv("APP_ENV") == "development" {
			// Usuario fake para que todo funcione
			claims := &Claims{
				UserID: 1,
				Email:  "dev@local",
				Role:   "admin",
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 🔒 MODO REAL (producción)
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "token requerido", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "formato inválido", http.StatusUnauthorized)
			return
		}

		claims, err := ValidateAccessToken(parts[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ── Obtener claims ──
func GetClaims(r *http.Request) *Claims {
	claims, ok := r.Context().Value(ClaimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// ── Autorización por rol ──
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			claims := GetClaims(r)
			if claims == nil {
				http.Error(w, "no autenticado", http.StatusUnauthorized)
				return
			}

			for _, role := range roles {
				if claims.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "acceso denegado", http.StatusForbidden)
		})
	}
}
