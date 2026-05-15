package middleware

import (
	"net/http"

	"github.com/neocode96/libsau/internal/auth"
)

func RequirePageAuth(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		tokenCookie, err := r.Cookie("access_token")

		// ❌ no cookie
		if err != nil || tokenCookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// 🔒 VALIDACIÓN CENTRALIZADA
		_, err = auth.ValidateAccessToken(tokenCookie.Value)

		// ❌ token inválido/expirado
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// ✅ acceso permitido
		next.ServeHTTP(w, r)
	})
}
