package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnvironmentVariables() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("no .env file found, assuming cloud environment...")
	}

	mandatory := []string{
		"PORT",
		"JWT_SECRET",
		"DB_BUSINESS_USER",
		"DB_BUSINESS_PASS",
		"DB_BUSINESS_HOST",
		"DB_BUSINESS_PORT",
		"EMAIL_SERVICE_EMAIL",
		"EMAIL_SERVICE_PASSWORD",
		"DOMAIN",
		"PORTAL_DOMAIN",
	}

	for _, k := range mandatory {
		if _, exists := os.LookupEnv(k); !exists {
			log.Fatalf("%s environment variable not set", k)
		}
	}
}