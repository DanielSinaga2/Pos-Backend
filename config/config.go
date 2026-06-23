package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort                  string
	AppBaseURL               string
	DBHost                   string
	DBPort                   string
	DBUser                   string
	DBPassword               string
	DBName                   string
	JWTSecret                string
	FrontendURL              string
	CorsAllowedOrigins       string
	RunSeeder                bool
	MidtransServerKey        string
	MidtransClientKey        string
	MidtransIsProduction     bool
	FrontendPaymentFinishURL string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	runSeeder, err := strconv.ParseBool(getEnv("RUN_SEEDER", "false"))
	if err != nil {
		return nil, fmt.Errorf("RUN_SEEDER must be true or false")
	}

	midtransIsProduction, err := strconv.ParseBool(getEnv("MIDTRANS_IS_PRODUCTION", "false"))
	if err != nil {
		return nil, fmt.Errorf("MIDTRANS_IS_PRODUCTION must be true or false")
	}

	frontendURL := getEnv("FRONTEND_URL", "http://localhost:3000")

	cfg := &Config{
		AppPort:                  getEnv("APP_PORT", "8080"),
		AppBaseURL:               getEnv("APP_BASE_URL", "http://localhost:8080"),
		DBHost:                   getEnv("DB_HOST", "localhost"),
		DBPort:                   getEnv("DB_PORT", "5432"),
		DBUser:                   getEnv("DB_USER", "postgres"),
		DBPassword:               os.Getenv("DB_PASSWORD"),
		DBName:                   getEnv("DB_NAME", "pos_restaurant"),
		JWTSecret:                os.Getenv("JWT_SECRET"),
		FrontendURL:              frontendURL,
		CorsAllowedOrigins:       getEnv("CORS_ALLOWED_ORIGINS", frontendURL),
		RunSeeder:                runSeeder,
		MidtransServerKey:        os.Getenv("MIDTRANS_SERVER_KEY"),
		MidtransClientKey:        os.Getenv("MIDTRANS_CLIENT_KEY"),
		MidtransIsProduction:     midtransIsProduction,
		FrontendPaymentFinishURL: getEnv("FRONTEND_PAYMENT_FINISH_URL", "http://localhost:3000/payment/finish"),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
