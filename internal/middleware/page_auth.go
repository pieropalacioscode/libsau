package middleware

import (
	"net/http"
	"strings"

	"github.com/neocode96/libsau/internal/auth"
)

// RequirePageAuth protege rutas HTML.
// Si no hay token válido en el header → redirige a /login.
// El frontend también valida con sessionStorage, esto es una segunda capa.
func RequirePageAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentar leer token del header Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				if _, err := auth.ValidateAccessToken(parts[1]); err == nil {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// Sin token válido en header: dejar pasar igualmente
		// (el JS del frontend maneja la redirección al login)
		// Si quisieras un redirect server-side: http.Redirect(w, r, "/login", 302)
		next.ServeHTTP(w, r)
	})
}