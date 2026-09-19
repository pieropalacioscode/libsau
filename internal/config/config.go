package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// JWTSecret se expone para que otros paquetes lo lean sin llamar a os.Getenv.
var JWTSecret string

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env no encontrado, usando variables del sistema")
	}

	// Validar que todas las variables críticas estén presentes.
	required := []string{"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "JWT_SECRET"}
	for _, key := range required {
		if os.Getenv(key) == "" {
			log.Fatalf("❌ Variable de entorno requerida '%s' no está definida", key)
		}
	}

	JWTSecret = os.Getenv("JWT_SECRET")
}
