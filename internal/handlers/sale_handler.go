package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/models"
)

type SaleHandler struct {
	db *gorm.DB
}

func NewSaleHandler(db *gorm.DB) *SaleHandler {
	return &SaleHandler{db: db}
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type SaleItemRequest struct {
	ProductID uint    `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Discount  float64 `json:"discount"`
}

type CreateSaleRequest struct {
	PayMethod string            `json:"pay_method"`
	Notes     *string           `json:"notes"`
	Items     []SaleItemRequest `json:"items"`
}

// ─── POST /api/v1/sales ───────────────────────────────────────────────────────
// Transacción atómica: valida stock → descuenta → registra venta.
// Si falla cualquier paso → ROLLBACK total. Stock nunca queda en estado inválido.

func (h *SaleHandler) Create(w http.ResponseWriter, r *http.Request) {
	// ── 1. Leer quién está vendiendo (del JWT) ────────────────────────────────
	claims := auth.GetClaims(r)
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "no autenticado")
		return
	}

	// ── 2. Parsear y validar request ──────────────────────────────────────────
	var req CreateSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	validPayMethods := map[string]bool{
		"EFECTIVO": true, "YAPE": true, "PLIN": true, "TARJETA": true,
	}
	if !validPayMethods[req.PayMethod] {
		respondError(w, http.StatusBadRequest, "pay_method inválido: usa EFECTIVO, YAPE, PLIN o TARJETA")
		return
	}

	if len(req.Items) == 0 {
		respondError(w, http.StatusBadRequest, "la venta debe tener al menos 1 ítem")
		return
	}

	for i, item := range req.Items {
		if item.ProductID == 0 {
			respondError(w, http.StatusBadRequest,
				"items["+strconv.Itoa(i)+"]: product_id es requerido")
			return
		}
		if item.Quantity <= 0 {
			respondError(w, http.StatusBadRequest,
				"items["+strconv.Itoa(i)+"]: quantity debe ser > 0")
			return
		}
		if item.Discount < 0 {
			respondError(w, http.StatusBadRequest,
				"items["+strconv.Itoa(i)+"]: discount no puede ser negativo")
			return
		}
	}

	// ── 3. TRANSACCIÓN ATÓMICA ────────────────────────────────────────────────
	// Todo lo que sigue ocurre dentro de una sola transacción PG.
	// Si cualquier paso falla → ROLLBACK automático.
	var sale models.Sale

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var saleItems []models.SaleItem
		var totalVenta float64

		for i, itemReq := range req.Items {
			// ── 3a. SELECT FOR UPDATE en el producto ──────────────────────────
			// NOWAIT: si otra transacción tiene el lock, falla inmediatamente
			// con error en lugar de esperar (evita bloquear el POS).
			// En producción: manejar el error NOWAIT con retry (ver D07).
			var product models.Product
			result := tx.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "NOWAIT", // falla si hay lock → evita espera indefinida
			}).First(&product, itemReq.ProductID)

			if result.Error != nil {
				if errors.Is(result.Error, gorm.ErrRecordNotFound) {
					return &saleError{
						field:   "items[" + strconv.Itoa(i) + "].product_id",
						message: "producto no encontrado",
						status:  http.StatusNotFound,
					}
				} else if strings.Contains(result.Error.Error(), "could not obtain lock") {
					return &saleError{
						field:   "items[" + strconv.Itoa(i) + "].product_id",
						message: "producto ocupado, reintenta",
						status:  http.StatusServiceUnavailable,
					}
				}

				return result.Error
			}

			if !product.Active {
				return &saleError{
					field:   "items[" + strconv.Itoa(i) + "].product_id",
					message: "producto '" + product.Name + "' no está activo",
					status:  http.StatusConflict,
				}
			}

			// ── 3b. Verificar stock suficiente ────────────────────────────────
			if product.Stock < itemReq.Quantity {
				return &saleError{
					field: "items[" + strconv.Itoa(i) + "].quantity",
					message: "stock insuficiente para '" + product.Name +
						"'. Disponible: " + strconv.Itoa(product.Stock) +
						", solicitado: " + strconv.Itoa(itemReq.Quantity),
					status: http.StatusConflict,
				}
			}

			// ── 3c. Validar que el descuento no supera el precio ──────────────
			if itemReq.Discount > product.Price {
				return &saleError{
					field:   "items[" + strconv.Itoa(i) + "].discount",
					message: "descuento no puede superar el precio del producto",
					status:  http.StatusBadRequest,
				}
			}

			// ── 3d. Descontar stock ───────────────────────────────────────────
			// UPDATE directo con condición de seguridad extra: stock >= quantity.
			// La condición en WHERE es la segunda línea de defensa contra negativos.
			updated := tx.Model(&models.Product{}).
				Where("id = ? AND stock >= ?", product.ID, itemReq.Quantity).
				UpdateColumn("stock", gorm.Expr("stock - ?", itemReq.Quantity))

			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 0 {
				// Esto solo pasa en condición de carrera extrema
				return &saleError{
					field:   "items[" + strconv.Itoa(i) + "]",
					message: "conflicto de stock en '" + product.Name + "'. Reintenta la venta.",
					status:  http.StatusConflict,
				}
			}

			// ── 3e. Construir ítem con snapshots de precio y costo ────────────
			item := models.SaleItem{
				ProductID:     product.ID,
				Quantity:      itemReq.Quantity,
				PriceSnapshot: product.Price, // snapshot: precio en este momento
				CostSnapshot:  product.Cost,  // snapshot: costo en este momento
				Discount:      itemReq.Discount,
			}
			item.CalculateSubtotal()

			saleItems = append(saleItems, item)
			totalVenta += item.Subtotal
		}

		// ── 3f. Crear la venta con todos los ítems ────────────────────────────
		sale = models.Sale{
			UserID:    claims.UserID,
			Origin:    "LOCAL",
			PayMethod: req.PayMethod,
			Status:    "COMPLETED",
			Total:     totalVenta,
			Notes:     req.Notes,
			Items:     saleItems,
		}

		// GORM crea la venta + todos los SaleItems en una sola operación
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}

		return nil // nil = COMMIT
	})

	// ── 4. Manejar resultado de la transacción ────────────────────────────────
	if err != nil {
		if sErr, ok := err.(*saleError); ok {
			respondJSON(w, sErr.status, map[string]string{
				"error": sErr.message,
				"field": sErr.field,
			})
			return
		}
		respondError(w, http.StatusInternalServerError, "error interno al procesar la venta")
		return
	}

	// ── 5. Cargar la venta completa con relaciones para la respuesta ──────────
	h.db.
		Preload("Items.Product.Category").
		Preload("Items.Product").
		Preload("User").
		First(&sale, sale.ID)
	respondJSON(w, http.StatusCreated, sale)
}

// ─── GET /api/v1/sales ────────────────────────────────────────────────────────

func (h *SaleHandler) List(w http.ResponseWriter, r *http.Request) {
	var sales []models.Sale

	query := h.db.
		Preload("Items.Product.Category").
		Preload("Items.Product").
		Preload("User").
		Order("created_at DESC")
	// Filtro opcional por método de pago: GET /sales?pay_method=YAPE
	if pm := r.URL.Query().Get("pay_method"); pm != "" {
		query = query.Where("pay_method = ?", pm)
	}

	if err := query.Find(&sales).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "error listando ventas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  sales,
		"total": len(sales),
	})
}

// ─── GET /api/v1/sales/{id} ───────────────────────────────────────────────────

func (h *SaleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var sale models.Sale
	err = h.db.Preload("Items.Product").Preload("User").First(&sale, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, http.StatusNotFound, "venta no encontrada")
			return
		}
		respondError(w, http.StatusInternalServerError, "error buscando venta")
		return
	}

	respondJSON(w, http.StatusOK, sale)
}

// ─── Error tipado para la transacción ────────────────────────────────────────
// Permite distinguir entre errores de negocio (409/404) y errores de sistema (500)

type saleError struct {
	field   string
	message string
	status  int
}

func (e *saleError) Error() string { return e.message }
