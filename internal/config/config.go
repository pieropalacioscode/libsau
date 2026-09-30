package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// JWTSecret almacena la clave en memoria para que el paquete auth la consuma
var JWTSecret string

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env no encontrado, usando variables del sistema")
	}

	// Validar variables críticas
	required := []string{"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "JWT_SECRET"}
	for _, key := range required {
		if os.Getenv(key) == "" {
			log.Printf("⚠️  Variable de entorno '%s' no definida", key)
		}
	}

	// Exportar el valor para que jwt.go y otros paquetes puedan usarlo
	JWTSecret = os.Getenv("JWT_SECRET")
}
