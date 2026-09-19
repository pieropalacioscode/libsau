package middleware

import (
	"net/http"

	"github.com/neocode96/libsau/internal/auth"
)

// RequirePageAuth protege rutas HTML que sirven vistas.
// Lee el token desde la cookie "access_token"; si no existe o es inválido
// redirige al login. Para rutas de API usa RequireAPIAuth (Bearer header).
func RequirePageAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if _, err = auth.ValidateAccessToken(cookie.Value); err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
