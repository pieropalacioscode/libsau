package auth

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/neocode96/libsau/internal/config"
)

// Regresión: el secret debe leerse al usar, no al arrancar el paquete.
func TestTokenSignedWithConfiguredSecret(t *testing.T) {
	// 1. Inyectamos la variable exportada en el test
	config.JWTSecret = "clave-de-prueba-123"

	tok, err := GenerateAccessToken(1, "a@b.c", "admin")
	if err != nil {
		t.Fatalf("no se pudo generar el token: %v", err)
	}

	// 2. Comprobamos la firma manualmente
	_, err = jwt.ParseWithClaims(tok, &Claims{}, func(*jwt.Token) (any, error) {
		return []byte("clave-de-prueba-123"), nil
	})
	if err != nil {
		t.Fatalf("el token NO está firmado con config.JWTSecret: %v", err)
	}

	// 3. Comprobamos que el propio validador lo acepte
	if _, err := ValidateAccessToken(tok); err != nil {
		t.Fatalf("ValidateAccessToken rechazó su propio token: %v", err)
	}
}
