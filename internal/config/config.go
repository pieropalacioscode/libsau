package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

var JWTSecret string

func Load() {

	_ = godotenv.Load()

	JWTSecret = os.Getenv("JWT_SECRET")

	if JWTSecret == "" {
		log.Fatal("JWT_SECRET no está definido")
	}
}
