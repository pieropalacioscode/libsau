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

// ───── LOGIN ─────
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	rateKey := fmt.Sprintf("rl:login:%s", req.Email)

	val, err := h.rdb.Get(ctx, rateKey).Result()

	attempts := 0
	if err == nil {
		fmt.Sscanf(val, "%d", &attempts)
	}
	if attempts >= 5 {
		http.Error(w, "demasiados intentos fallidos", http.StatusTooManyRequests)
		return
	}

	var user models.User
	if err := h.db.Where("email = ? AND active = true", req.Email).First(&user).Error; err != nil {
		h.incrementAttempts(ctx, rateKey)
		http.Error(w, "credenciales inválidas", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.incrementAttempts(ctx, rateKey)
		http.Error(w, "credenciales inválidas", http.StatusUnauthorized)
		return
	}

	// ✅ LOGIN CORRECTO → resetear contador
	h.rdb.Del(ctx, rateKey)

	accessToken, _ := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	refreshToken, _ := auth.GenerateRefreshToken(user.ID)

	key := fmt.Sprintf("refresh:user:%d", user.ID)
	h.rdb.Set(ctx, key, refreshToken, 7*24*time.Hour)

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HttpOnly: true,
		Path:     "/auth/refresh",
		MaxAge:   7 * 24 * 60 * 60,
	})

	json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken,
	})
}

// ───── REFRESH ─────
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()
	keys, _ := h.rdb.Keys(ctx, "refresh:user:*").Result()

	var userID uint

	for _, key := range keys {
		val, _ := h.rdb.Get(ctx, key).Result()
		if val == cookie.Value {
			fmt.Sscanf(key, "refresh:user:%d", &userID)
			break
		}
	}

	if userID == 0 {
		http.Error(w, "token inválido", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "usuario no encontrado", http.StatusUnauthorized)
		return
	}

	newToken, _ := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role))

	json.NewEncoder(w).Encode(map[string]string{
		"access_token": newToken,
	})
}

// ───── LOGOUT ─────
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)

	if claims != nil {
		key := fmt.Sprintf("refresh:user:%d", claims.UserID)
		h.rdb.Del(context.Background(), key)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "refresh_token",
		Value:  "",
		MaxAge: -1,
		Path:   "/auth/refresh",
	})

	json.NewEncoder(w).Encode(map[string]string{
		"message": "logout ok",
	})
}

// ───── ME ─────
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)

	if claims == nil {
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(claims)
}

// ───── HELPER ─────
func (h *AuthHandler) incrementAttempts(ctx context.Context, key string) {
	pipe := h.rdb.TxPipeline()

	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)

	pipe.Exec(ctx)

	fmt.Println("Intentos:", incr.Val()) // DEBUG 🔥
}
