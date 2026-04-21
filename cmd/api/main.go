package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/neocode96/libsau/internal/auth"
	"github.com/neocode96/libsau/internal/database"
	"github.com/neocode96/libsau/internal/handlers"
	"github.com/neocode96/libsau/internal/models"
)

func main() {
	// ── ENV ──
	_ = godotenv.Load()

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	// ── DB ──
	db, err := database.Connect()
	if err != nil {
		log.Fatal("❌ Error conectando a DB:", err)
	}

	log.Println("🔄 Ejecutando migraciones...")
	err = db.AutoMigrate(
		&models.Category{},
		&models.Product{},
		&models.User{},
		&models.Sale{},
		&models.SaleItem{},
	)
	if err != nil {
		log.Fatal("❌ Error en AutoMigrate:", err)
	}

	// ── Redis ──
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_HOST") + ":" + os.Getenv("REDIS_PORT"),
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("❌ Redis no conectado:", err)
	}
	log.Println("✅ Redis conectado")

	// ── Handlers ──
	productHandler := handlers.NewProductHandler(db)
	categoryHandler := handlers.NewCategoryHandler(db)
	authHandler := handlers.NewAuthHandler(db, rdb)
	saleHandler := handlers.NewSaleHandler(db)
	// ── Router ──
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// ── HEALTH ──
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
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

	// ── API PRIVADA ──
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Authenticate)

		r.Route("/categories", func(r chi.Router) {
			r.Get("/", categoryHandler.List)
			r.Post("/", categoryHandler.Create)
		})

		r.Route("/products", func(r chi.Router) {
			r.Get("/", productHandler.List)
			r.Post("/", productHandler.Create)
			r.Get("/{id}", productHandler.Get)
		})
		r.Route("/sales", func(r chi.Router) {
			r.Post("/", saleHandler.Create)
			r.Get("/", saleHandler.List)
			r.Get("/{id}", saleHandler.Get)
		})
	})

	// ── AUTH (correcto) ──
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.With(auth.Authenticate).Post("/logout", authHandler.Logout)
		r.With(auth.Authenticate).Get("/me", authHandler.Me)
	})

	// ── SERVER ──
	log.Println("🚀 LIBSAU corriendo en :" + port)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
