package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/neocode96/libsau/internal/models"
)

type WooWorker struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewWooWorker(db *gorm.DB, rdb *redis.Client) *WooWorker {
	return &WooWorker{db: db, rdb: rdb}
}

// Run es una goroutine permanente. Consume la cola Redis "woo:orders".
// Se llama con: go worker.Run(ctx)
func (w *WooWorker) Run(ctx context.Context) {
	log.Println("🚀 WooWorker iniciado — escuchando cola woo:orders")

	for {
		select {
		case <-ctx.Done():
			log.Println("⏹  WooWorker detenido")
			return
		default:
		}

		// BLPOP bloquea hasta que haya un item (timeout 5s para chequear ctx)
		result, err := w.rdb.BRPop(ctx, 5*time.Second, "woo:orders").Result()
		if err != nil {
			// Timeout normal → volver a esperar
			continue
		}

		// result[0] = nombre de la lista, result[1] = payload
		if len(result) < 2 {
			continue
		}

		var order WooOrder
		if err := json.Unmarshal([]byte(result[1]), &order); err != nil {
			log.Printf("❌ WooWorker: JSON inválido: %v", err)
			continue
		}

		log.Printf("🔄 WooWorker: procesando order WOO #%d", order.ID)

		if err := w.processOrder(ctx, &order); err != nil {
			log.Printf("❌ WooWorker: error en order #%d: %v", order.ID, err)
			// En producción: mover a cola de errores con backoff
			// Por ahora: loguear y continuar
		}
	}
}

// processOrder crea la venta en estado PENDING_CONFIRM.
// NO descuenta stock todavía — eso lo hace el vendedor al confirmar.
func (w *WooWorker) processOrder(ctx context.Context, order *WooOrder) error {
	externalID := strconv.Itoa(order.ID)

	// ── Idempotencia nivel BD ─────────────────────────────────────────────────
	// Aunque Redis ya filtra por delivery-id, WooCommerce puede cambiar el
	// delivery-id en reintentos. La UNIQUE constraint en external_id es la
	// segunda línea de defensa que garantiza que nunca creamos duplicados.
	var existing models.Sale
	err := w.db.Where("external_id = ?", externalID).First(&existing).Error
	if err == nil {
		log.Printf("ℹ️  WooWorker: order #%d ya existe como venta #%d", order.ID, existing.ID)
		return nil
	}

	// ── Mapear line_items → productos locales ────────────────────────────────
	type resolvedItem struct {
		product  models.Product
		quantity int
		price    float64
		lineName string
	}

	var resolved []resolvedItem
	var unmapped []string

	for _, line := range order.LineItems {
		if line.Quantity <= 0 {
			continue
		}

		var product models.Product
		// Estrategia de mapeo (en orden de confianza):
		// 1. Por SKU exacto si WooCommerce lo envía
		// 2. Por ID externo en metadata (a futuro con tabla producto_canal)
		// 3. Por nombre similar (fallback)
		found := false

		if line.SKU != "" {
			if err := w.db.Where("sku = ? AND active = ?", line.SKU, true).
				First(&product).Error; err == nil {
				found = true
			}
		}

		if !found {
			// Intentar por nombre (búsqueda aproximada)
			if err := w.db.Where("name ILIKE ? AND active = ?",
				"%"+line.Name+"%", true).First(&product).Error; err == nil {
				found = true
			}
		}

		if !found {
			unmapped = append(unmapped, fmt.Sprintf("%s (qty:%d)", line.Name, line.Quantity))
			continue
		}

		price, _ := strconv.ParseFloat(line.Price, 64)
		resolved = append(resolved, resolvedItem{
			product:  product,
			quantity: line.Quantity,
			price:    price,
			lineName: line.Name,
		})
	}

	// ── Construir nota con ítems no mapeados ──────────────────────────────────
	notes := fmt.Sprintf("Pedido WooCommerce #%d", order.ID)
	if len(unmapped) > 0 {
		notes += fmt.Sprintf(" | SIN MAPEAR: %s", strings.Join(unmapped, ", "))
	}

	// ── Nombre completo del cliente ───────────────────────────────────────────
	customerName := strings.TrimSpace(order.Billing.FirstName + " " + order.Billing.LastName)

	// ── Dirección de envío ────────────────────────────────────────────────────
	addr := order.Shipping.Address1
	if order.Shipping.Address2 != "" {
		addr += ", " + order.Shipping.Address2
	}
	if order.Shipping.City != "" {
		addr += ", " + order.Shipping.City
	}

	totalWoo, _ := strconv.ParseFloat(order.Total, 64)

	// ── Crear la venta en estado PENDIENTE ────────────────────────────────────
	// IMPORTANTE: en este punto NO hay SELECT FOR UPDATE ni descuento de stock.
	// El stock se descuenta SOLO cuando el vendedor confirma físicamente.
	return w.db.Transaction(func(tx *gorm.DB) error {
		sale := models.Sale{
			Origin:        "WOO",
			PayMethod:     "WEB",
			Status:        "PENDING_CONFIRM", // ← espera confirmación del vendedor
			Total:         totalWoo,
			Notes:         &notes,
			ExternalID:    &externalID,
			CustomerName:  &customerName,
			CustomerPhone: &order.Billing.Phone,
			ShippingAddr:  &addr,
		}

		if err := tx.Create(&sale).Error; err != nil {
			return fmt.Errorf("creando venta: %w", err)
		}

		// Crear ítems (sin afectar stock todavía)
		for _, item := range resolved {
			si := models.SaleItem{
				SaleID:        sale.ID,
				ProductID:     item.product.ID,
				Quantity:      item.quantity,
				PriceSnapshot: item.price,
				CostSnapshot:  item.product.Cost,
				Discount:      0,
			}
			si.CalculateSubtotal()

			if err := tx.Create(&si).Error; err != nil {
				return fmt.Errorf("creando item: %w", err)
			}
		}

		log.Printf("✅ WooWorker: order WOO #%d → venta local #%d creada (PENDING_CONFIRM)",
			order.ID, sale.ID)

		// Publicar evento en Redis para notificar al panel en tiempo real
		event := map[string]any{
			"type":         "new_woo_order",
			"sale_id":      sale.ID,
			"woo_order_id": order.ID,
			"customer":     customerName,
			"total":        totalWoo,
			"items_count":  len(resolved),
			"unmapped":     len(unmapped),
		}
		eventJSON, _ := json.Marshal(event)
		w.rdb.Publish(ctx, "libsau:events", eventJSON)

		return nil
	})
}

// ── ConfirmSale: vendedor confirma que sí hay stock ───────────────────────────
// AQUÍ es donde se hace el SELECT FOR UPDATE y se descuenta el stock.

func (w *WooWorker) ConfirmSale(saleID uint, userID uint) error {
	return w.db.Transaction(func(tx *gorm.DB) error {
		// Cargar la venta con lock
		var sale models.Sale
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Items").First(&sale, saleID).Error; err != nil {
			return err
		}

		if sale.Status != "PENDING_CONFIRM" {
			return fmt.Errorf("venta #%d no está en estado PENDING_CONFIRM (estado: %s)",
				saleID, sale.Status)
		}

		// Para cada ítem: verificar y descontar stock
		for _, item := range sale.Items {
			var product models.Product
			if err := tx.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "NOWAIT",
			}).First(&product, item.ProductID).Error; err != nil {
				return fmt.Errorf("bloqueando producto %d: %w", item.ProductID, err)
			}

			if product.Stock < item.Quantity {
				return fmt.Errorf("stock insuficiente para '%s': disponible %d, requerido %d",
					product.Name, product.Stock, item.Quantity)
			}

			// Descontar stock con condición de seguridad extra
			result := tx.Model(&models.Product{}).
				Where("id = ? AND stock >= ?", product.ID, item.Quantity).
				UpdateColumn("stock", gorm.Expr("stock - ?", item.Quantity))

			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("conflicto de stock en '%s', reintenta", product.Name)
			}
		}

		// Actualizar estado de la venta
		if err := tx.Model(&sale).Updates(map[string]any{
			"status":  "COMPLETED",
			"user_id": userID,
		}).Error; err != nil {
			return err
		}

		log.Printf("✅ Venta WOO #%d confirmada por usuario #%d — stock descontado", saleID, userID)
		return nil
	})
}
