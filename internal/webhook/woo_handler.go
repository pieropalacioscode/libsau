package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ─── Payload de WooCommerce (order.created / order.updated) ──────────────────
// Solo mapeamos lo que necesitamos. WooCommerce envía mucho más.

type WooOrder struct {
	ID            int           `json:"id"`
	Status        string        `json:"status"`
	Total         string        `json:"total"`
	PaymentMethod string        `json:"payment_method_title"`
	Billing       WooBilling    `json:"billing"`
	Shipping      WooShipping   `json:"shipping"`
	LineItems     []WooLineItem `json:"line_items"`
	DateCreated   string        `json:"date_created"`
}

type WooBilling struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
}

type WooShipping struct {
	Address1 string `json:"address_1"`
	Address2 string `json:"address_2"`
	City     string `json:"city"`
	State    string `json:"state"`
}

type WooLineItem struct {
	ID          int    `json:"id"`
	ProductID   int    `json:"product_id"`
	VariationID int    `json:"variation_id"`
	Name        string `json:"name"`
	Quantity    int    `json:"quantity"`
	Price       string `json:"price"`
	SKU         string `json:"sku"`
}

// ─── Handler ──────────────────────────────────────────────────────────────────

type WooWebhookHandler struct {
	db     *gorm.DB
	rdb    *redis.Client
	secret string
}

func NewWooWebhookHandler(db *gorm.DB, rdb *redis.Client) *WooWebhookHandler {
	return &WooWebhookHandler{
		db:     db,
		rdb:    rdb,
		secret: os.Getenv("WOO_WEBHOOK_SECRET"),
	}
}

// POST /webhooks/woocommerce
// Objetivo: responder en < 200ms siempre.
// Verificar firma → encolar en Redis → responder 200.
// El procesamiento real lo hace el Worker en background.

func (h *WooWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// ── 1. Leer body (máx 1MB) ────────────────────────────────────────────────
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	// ── 2. Verificar firma HMAC-SHA256 ────────────────────────────────────────
	// WooCommerce envía: X-Wc-Webhook-Signature: base64(HMAC-SHA256(body, secret))
	if h.secret != "" {
		sig := r.Header.Get("X-Wc-Webhook-Signature")
		if !h.verifySignature(body, sig) {
			log.Printf("🚨 Webhook WOO: firma inválida desde %s", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// ── 3. Solo procesamos order.created y order.updated ─────────────────────
	topic := r.Header.Get("X-Wc-Webhook-Topic")
	if topic != "order.created" && topic != "order.updated" {
		// Responder 200 y descartar silenciosamente (evitar reintentos de WOO)
		w.WriteHeader(http.StatusOK)
		return
	}

	// ── 4. Idempotencia por Delivery-ID ───────────────────────────────────────
	// WooCommerce envía un Delivery-ID único por cada intento de entrega.
	deliveryID := r.Header.Get("X-Wc-Webhook-Delivery-Id")
	if deliveryID != "" {
		ctx := context.Background()
		idemKey := fmt.Sprintf("woo:delivery:%s", deliveryID)
		set, err := h.rdb.SetNX(ctx, idemKey, "1", 48*time.Hour).Result()
		if err == nil && !set {
			// Ya procesamos este delivery — responder 200 sin reencolar
			log.Printf("📦 Webhook WOO: delivery %s ya procesado (idempotente)", deliveryID)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// ── 5. Validar que el JSON es una orden válida ────────────────────────────
	var order WooOrder
	if err := json.Unmarshal(body, &order); err != nil {
		log.Printf("⚠️ Webhook WOO: JSON inválido: %v", err)
		w.WriteHeader(http.StatusOK) // Responder 200 para evitar reintentos
		return
	}

	// Solo procesar órdenes en estado "processing" o "completed"
	if order.Status != "processing" && order.Status != "completed" {
		log.Printf("📦 Webhook WOO: order #%d en estado '%s' ignorada", order.ID, order.Status)
		w.WriteHeader(http.StatusOK)
		return
	}

	// ── 6. Encolar en Redis para procesamiento asíncrono ─────────────────────
	payload, _ := json.Marshal(order)
	if err := h.rdb.LPush(context.Background(), "woo:orders", payload).Err(); err != nil {
		log.Printf("❌ Webhook WOO: error encolando order #%d: %v", order.ID, err)
		// Responder 500 para que WooCommerce reintente
		http.Error(w, "queue error", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Webhook WOO: order #%d encolada (topic: %s)", order.ID, topic)

	// ── 7. Responder 200 inmediatamente ───────────────────────────────────────
	w.WriteHeader(http.StatusOK)
}

func (h *WooWebhookHandler) verifySignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
