package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/neocode96/libsau/internal/config"
)

type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

var ErrTokenExpired = errors.New("token expirado")

// 🔥 PROTECCIÓN: nunca permitir secret vacío
func secretBytes() []byte {
	if config.JWTSecret == "" {
		panic("JWTSecret no está configurado (revisar variables de entorno)")
	}
	return []byte(config.JWTSecret)
}

// ───── ACCESS TOKEN (15 min) ─────
func GenerateAccessToken(userID uint, email, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "libsau",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretBytes())
}

// ───── VALIDAR ACCESS TOKEN ─────
func ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		// 🔒 evitar ataques de algoritmo
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de firma inválido")
		}
		return secretBytes(), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, errors.New("token inválido")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token inválido")
	}

	// 🔥 VALIDACIONES EXTRA (IMPORTANTE)
	if claims.Issuer != "libsau" {
		return nil, errors.New("issuer inválido")
	}

	return claims, nil
}

// ───── REFRESH TOKEN (7 días) ─────
func GenerateRefreshToken(userID uint) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "libsau",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretBytes())
}

// ───── VALIDAR REFRESH TOKEN ─────
func ValidateRefreshToken(tokenStr string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		// 🔒 seguridad algoritmo
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return secretBytes(), nil
	})

	if err != nil {
		return nil, errors.New("refresh token inválido")
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, errors.New("refresh token inválido")
	}

	// 🔥 VALIDACIONES CRÍTICAS
	if claims.Issuer != "libsau" {
		return nil, errors.New("issuer inválido")
	}

	if claims.Subject == "" {
		return nil, errors.New("subject inválido")
	}

	return claims, nil
}

// ───── EXTRACCIÓN USER ID ─────
func ExtractUserIDFromRefresh(claims *jwt.RegisteredClaims) (uint, error) {
	var userID uint
	_, err := fmt.Sscanf(claims.Subject, "%d", &userID)
	if err != nil || userID == 0 {
		return 0, errors.New("userID inválido")
	}
	return userID, nil
}
