package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/models"
)

// ─────────────────────────────────────────────
// Constantes de configuración
// ─────────────────────────────────────────────

const (
	maxLoginAttempts   = 5
	rateLimitWindow    = 2 * time.Minute
	accessTokenMaxAge  = 60 * 15          // 15 min
	refreshTokenMaxAge = 60 * 60 * 24 * 7 // 7 días
	refreshTokenTTL    = 7 * 24 * time.Hour
	refreshCookiePath  = "/auth/refresh"
)

// ─────────────────────────────────────────────
// AuthHandler
// ─────────────────────────────────────────────

type AuthHandler struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewAuthHandler(db *gorm.DB, rdb *redis.Client) *AuthHandler {
	return &AuthHandler{db: db, rdb: rdb}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ─────────────────────────────────────────────
// Helpers HTTP
// ─────────────────────────────────────────────

// writeJSON serializa payload como JSON con el status dado.
// Siempre establece Content-Type antes de WriteHeader para evitar
// el warning de Go sobre headers ya enviados.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// httpErr centraliza los errores HTTP. Wrapper mínimo sobre
// http.Error para mantener la firma familiar de la stdlib.
func httpErr(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}

// ─────────────────────────────────────────────
// Helpers de Cookies
// ─────────────────────────────────────────────

// setAccessCookie emite la cookie del access token (15 min, path /).
// Compatible con SSR pages (HttpOnly) y silent refresh del frontend.
func setAccessCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   accessTokenMaxAge,
	})
}

// setRefreshCookie emite la cookie del refresh token (7 días).
// Path restringido a /auth/refresh por seguridad: el browser solo
// la envía a ese endpoint, reduciendo la superficie de ataque.
func setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     refreshCookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   refreshTokenMaxAge,
	})
}

// clearAuthCookies borra ambas cookies forzando MaxAge: -1.
// Los Path deben coincidir exactamente con los usados al crearlas,
// de lo contrario el browser ignora la instrucción de borrado.
func clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ─────────────────────────────────────────────
// Helpers de Redis (rate limiting + sesión)
// ─────────────────────────────────────────────

// sessionKey genera la clave Redis de sesión de forma centralizada.
// Unifica el formato para Login, Refresh y Logout (antes divergían
// entre %d y %s, lo que era frágil aunque funcionalmente equivalente).
func sessionKey(userID uint) string {
	return fmt.Sprintf("session:%d", userID)
}

func rateLimitKey(email string) string {
	return fmt.Sprintf("rl:login:%s", email)
}

// isRateLimited devuelve true si el email superó maxLoginAttempts.
func (h *AuthHandler) isRateLimited(ctx context.Context, key string) bool {
	val, err := h.rdb.Get(ctx, key).Result()
	if err != nil {
		return false // clave inexistente → primer intento
	}
	var attempts int
	fmt.Sscanf(val, "%d", &attempts)
	return attempts >= maxLoginAttempts
}

// incrementAttempts incrementa el contador de intentos fallidos
// con una ventana deslizante de rateLimitWindow.
func (h *AuthHandler) incrementAttempts(ctx context.Context, key string) {
	pipe := h.rdb.TxPipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rateLimitWindow)
	pipe.Exec(ctx)
}

// saveSession persiste el refresh token en Redis como whitelist.
// Permite revocar sesiones individuales en O(1) sin tocar la DB.
func (h *AuthHandler) saveSession(ctx context.Context, userID uint, refreshToken string) error {
	return h.rdb.Set(ctx, sessionKey(userID), refreshToken, refreshTokenTTL).Err()
}

// validateSession comprueba que el token coincide con el almacenado
// (whitelist). Si no coincide, la sesión fue revocada o el token es
// incorrecto (posible robo de cookie).
func (h *AuthHandler) validateSession(ctx context.Context, userID uint, token string) bool {
	stored, err := h.rdb.Get(ctx, sessionKey(userID)).Result()
	return err == nil && stored == token
}

// revokeSession elimina la sesión de Redis, invalidando el refresh
// token inmediatamente sin esperar a que el JWT expire.
func (h *AuthHandler) revokeSession(ctx context.Context, userID uint) {
	h.rdb.Del(ctx, sessionKey(userID))
}

// ─────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────

// Login autentica al usuario y emite access + refresh token.
//
// Flujo:
//  1. Decodificar body → validar rate limit → buscar usuario en DB
//  2. Comparar contraseña con bcrypt
//  3. Generar access token (JWT corto) + refresh token (JWT largo)
//  4. Guardar refresh token en Redis (whitelist de sesión)
//  5. Emitir ambas cookies HttpOnly + responder con access token en JSON
//     (el JSON permite que el frontend JS lo guarde en sessionStorage para
//     peticiones autenticadas sin leer la cookie HttpOnly)
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	ctx := r.Context() // propaga cancelaciones del request al cliente Redis/GORM
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	rateKey := rateLimitKey(req.Email)

	if h.isRateLimited(ctx, rateKey) {
		httpErr(w, http.StatusTooManyRequests, "demasiados intentos fallidos")
		return
	}

	var user models.User
	if err := h.db.Where("email = ? AND active = true", req.Email).First(&user).Error; err != nil {
		h.incrementAttempts(ctx, rateKey)
		httpErr(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.incrementAttempts(ctx, rateKey)
		httpErr(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}

	// Login correcto → limpiar contador de intentos
	h.rdb.Del(ctx, rateKey)

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "error generando access token")
		return
	}

	refreshToken, err := auth.GenerateRefreshToken(user.ID)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "error generando refresh token")
		return
	}

	if err := h.saveSession(ctx, user.ID, refreshToken); err != nil {
		httpErr(w, http.StatusInternalServerError, "error guardando sesión")
		return
	}

	setAccessCookie(w, accessToken)
	setRefreshCookie(w, refreshToken)

	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": accessToken,
	})
}

// Refresh emite un nuevo access token a partir de un refresh token válido.
//
// Flujo:
//  1. Leer cookie refresh_token → validar firma JWT
//  2. Consultar usuario en DB para obtener datos frescos (rol, etc.)
//  3. Verificar en Redis que la sesión no fue revocada (whitelist)
//  4. Generar y emitir nuevo access token
//
// Nota: el orden DB → Redis (vs. Redis → DB del login) es intencional:
// necesitamos user.ID (uint) para construir sessionKey de forma consistente
// sin importar strconv. El costo de un lookup extra es despreciable aquí.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		httpErr(w, http.StatusUnauthorized, "no hay token de refresco")
		return
	}

	claims, err := auth.ValidateRefreshToken(cookie.Value)
	if err != nil {
		httpErr(w, http.StatusUnauthorized, "token inválido o expirado")
		return
	}

	ctx := r.Context()

	// Datos frescos del usuario: captura cambios de rol en caliente
	// sin necesitar invalidar el token anterior manualmente.
	var user models.User
	if err := h.db.First(&user, claims.Subject).Error; err != nil {
		httpErr(w, http.StatusUnauthorized, "usuario no encontrado")
		return
	}

	// Whitelist check: si el token no coincide, la sesión fue revocada
	// (logout explícito o rotación de tokens por seguridad).
	if !h.validateSession(ctx, user.ID, cookie.Value) {
		httpErr(w, http.StatusUnauthorized, "sesión expirada o revocada")
		return
	}

	newToken, err := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "error generando access token")
		return
	}

	setAccessCookie(w, newToken)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": newToken,
	})
}

// Logout revoca la sesión y borra ambas cookies.
//
// La revocación en Redis es best-effort: si falla, el access token
// expira solo en 15 min y el refresh token quedará huérfano en Redis
// hasta que TTL lo limpie. Aceptable para este caso de uso.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r); claims != nil {
		h.revokeSession(r.Context(), claims.UserID)
	}

	clearAuthCookies(w)

	// HX-Redirect debe establecerse antes de writeJSON porque
	// writeJSON llama a WriteHeader, que congela los headers.
	w.Header().Set("HX-Redirect", "/login")

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "sesión cerrada correctamente",
	})
}

// Me devuelve los claims del access token del request actual.
// El middleware ya validó el token; aquí solo lo exponemos.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	if claims == nil {
		httpErr(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	writeJSON(w, http.StatusOK, claims)
}
