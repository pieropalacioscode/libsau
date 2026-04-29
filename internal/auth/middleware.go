package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// ─────────────────────────────────────────
// Helper: SIEMPRE responder JSON
// ─────────────────────────────────────────
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
	})
}

// ─────────────────────────────────────────
// Context
// ─────────────────────────────────────────
type contextKey string

const ClaimsKey contextKey = "claims"

// ─────────────────────────────────────────
// Middleware: Authenticate
// ─────────────────────────────────────────
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// 🧠 MODO DEV (IMPORTANTE)
		if os.Getenv("APP_ENV") == "development" {
			claims := &Claims{
				UserID: 1,
				Email:  "dev@local",
				Role:   "admin",
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 🔒 TOKEN
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "token requerido")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeJSONError(w, http.StatusUnauthorized, "formato inválido")
			return
		}

		claims, err := ValidateAccessToken(parts[1])
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ─────────────────────────────────────────
// Obtener claims
// ─────────────────────────────────────────
func GetClaims(r *http.Request) *Claims {
	claims, ok := r.Context().Value(ClaimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// ─────────────────────────────────────────
// Middleware: RequireRole
// ─────────────────────────────────────────
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			claims := GetClaims(r)
			if claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "no autenticado")
				return
			}

			for _, role := range roles {
				if claims.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeJSONError(w, http.StatusForbidden, "acceso denegado")
		})
	}
}
