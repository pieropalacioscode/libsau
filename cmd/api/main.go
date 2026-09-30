// cmd/api/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/config"
	"github.com/neocode96/libsau/internal/database"
	"github.com/neocode96/libsau/internal/handlers"
	"github.com/neocode96/libsau/internal/middleware"
	"github.com/neocode96/libsau/internal/models"
	"github.com/neocode96/libsau/internal/webhook"
)

func main() {
	// ── Config y entorno ───────────────────────────────────────────────────────
	config.Load()

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	// ── PostgreSQL ─────────────────────────────────────────────────────────────
	db, err := database.Connect()
	if err != nil {
		log.Fatal("❌ PostgreSQL:", err)
	}

	log.Println("🔄 Ejecutando migraciones...")

	// Fase 1: Business debe ir primero para que las FKs funcionen
	if err := db.AutoMigrate(
		&models.Business{},
		&models.Category{},
		&models.Product{},
		&models.ProductAttribute{},
		&models.User{},
		&models.Sale{},
		&models.SaleItem{},
		&models.CashClose{},
		// &models.StockAdjustmentLog{}, // 🔥 Comentado hasta que el modelo se cree en internal/models
	); err != nil {
		log.Fatal("❌ AutoMigrate:", err)
	}

	// Sembrar el negocio inicial si no existe
	var businessCount int64
	db.Model(&models.Business{}).Count(&businessCount)
	if businessCount == 0 {
		db.Create(&models.Business{Slug: "libreria-saber", Name: "Librería Saber", Tier: "FULL", Active: true})
		log.Println("✅ Negocio inicial creado: Librería Saber")
	}

	// if err := migrate(db); err != nil {
	//  log.Fatal("❌ Error en migraciones manuales:", err)
	// }
	// log.Println("✅ Tablas migradas")

	// ── Redis ──────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_HOST") + ":" + os.Getenv("REDIS_PORT"),
		Password: os.Getenv("REDIS_PASSWORD"),
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("❌ Redis:", err)
	}
	log.Println("✅ Redis conectado")

	// ── Índice GIN para búsqueda fuzzy ─────────────────────────────────────────
	db.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm;`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_products_name_trgm
             ON products USING GIN(name gin_trgm_ops);`)

	// ── Handlers ───────────────────────────────────────────────────────────────
	authHandler := handlers.NewAuthHandler(db, rdb)
	productHandler := handlers.NewProductHandler(db)
	categoryHandler := handlers.NewCategoryHandler(db)
	saleHandler := handlers.NewSaleHandler(db)
	searchHandler := handlers.NewSearchHandler(db)
	dashboardHandler := handlers.NewDashboardHandler(db)
	cashHandler := handlers.NewCashHandler(db)
	publicHandler := handlers.NewPublicHandler(db)

	// ── WooCommerce ────────────────────────────────────────────────────────────
	wooHandler := webhook.NewWooWebhookHandler(db, rdb)
	wooWorker := webhook.NewWooWorker(db, rdb)
	wooConfirmHandler := handlers.NewWooConfirmHandler(db, wooWorker)

	// Worker en background (escucha cola Redis "woo:orders")
	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wooWorker.Run(bgCtx)

	// ── Router ─────────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// ── Archivos estáticos ─────────────────────────────────────────────────────
	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// ── Health check ───────────────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		status := map[string]string{"status": "ok"}
		if err := database.Ping(); err != nil {
			status["postgres"] = "down"
			status["status"] = "degraded"
		} else {
			status["postgres"] = "up"
		}
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			status["redis"] = "down"
			status["status"] = "degraded"
		} else {
			status["redis"] = "up"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// ── Webhook WooCommerce — PÚBLICO (sin JWT) ────────────────────────────────
	r.Post("/webhooks/woocommerce", wooHandler.Handle)

	// ── API pública de catálogo — PÚBLICO (sin JWT), por slug de negocio ───────
	// Fase 6. Fuera del grupo /api/v1 a propósito: ni Authenticate ni
	// ResolveBusiness aplican aquí; cada handler resuelve el slug por su cuenta.
	r.Route("/public/{slug}", func(r chi.Router) {
		r.Get("/", publicHandler.Business)
		r.Get("/productos", publicHandler.ListProducts)
		r.Get("/productos/{id}", publicHandler.GetProduct)
		r.Get("/categorias", publicHandler.ListCategories)
	})

	// ── Páginas HTML ───────────────────────────────────────────────────────────
	r.Get("/", handlers.PageHome)
	r.Get("/login", handlers.PageLogin)
	r.With(middleware.RequirePageAuth).Get("/pos", handlers.PagePOS)
	// r.With(middleware.RequirePageAuth).Get("/dashboard", handlers.PageDashboard) // 🔥 Comentado hasta crear el handler de la vista HTML
	r.With(middleware.RequirePageAuth).Get("/cash", handlers.PageCash)
	r.With(middleware.RequirePageAuth).Get("/products", handlers.PageProducts)

	// ── Auth — público ─────────────────────────────────────────────────────────
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.With(auth.Authenticate).Post("/logout", authHandler.Logout)
		r.With(auth.Authenticate).Get("/me", authHandler.Me)
	})

	// ── API privada — requiere JWT ─────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Authenticate)
		r.Use(middleware.ResolveBusiness(db))

		r.Route("/categories", func(r chi.Router) {
			r.Get("/", categoryHandler.List)
			r.With(auth.RequireRole("admin")).Post("/", categoryHandler.Create)
		})

		r.Route("/products", func(r chi.Router) {
			r.Get("/search", searchHandler.Search)
			r.Get("/", productHandler.List)
			r.Get("/{id}", productHandler.Get)
			r.With(auth.RequireRole("admin")).Post("/", productHandler.Create)
			r.With(auth.RequireRole("admin")).Patch("/{id}", productHandler.Update)
			r.With(auth.RequireRole("admin")).Patch("/{id}/stock", productHandler.AdjustStock)
			r.With(auth.RequireRole("admin")).Delete("/{id}", productHandler.Delete)
		})

		r.Route("/sales", func(r chi.Router) {
			r.With(auth.RequireRole("admin", "vendedor")).Post("/", saleHandler.Create)
			r.With(auth.RequireRole("admin", "vendedor")).Get("/", saleHandler.List)
			r.With(auth.RequireRole("admin", "vendedor")).Get("/pending", wooConfirmHandler.ListPending)
			r.With(auth.RequireRole("admin", "vendedor")).Get("/{id}", saleHandler.Get)
			r.With(auth.RequireRole("admin", "vendedor")).Patch("/{id}/confirm", wooConfirmHandler.Confirm)
		})

		r.With(auth.RequireRole("admin")).Get("/dashboard", dashboardHandler.Get)

		r.Route("/cash", func(r chi.Router) {
			r.With(auth.RequireRole("admin", "vendedor")).Get("/today", cashHandler.Today)
			r.With(auth.RequireRole("admin")).Post("/close", cashHandler.Close)
			r.With(auth.RequireRole("admin")).Get("/history", cashHandler.History)
		})
	})

	log.Printf("🚀 LIBSAU corriendo en :%s", port)
	http.ListenAndServe(":"+port, r)
}
