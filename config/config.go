package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort     string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	JWTSecret   string
	FrontendURL string
	RunSeeder   bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	runSeeder, err := strconv.ParseBool(getEnv("RUN_SEEDER", "false"))
	if err != nil {
		return nil, fmt.Errorf("RUN_SEEDER must be true or false")
	}

	cfg := &Config{
		AppPort:     getEnv("APP_PORT", "8080"),
		DBHost:      getEnv("DB_HOST", "localhost"),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "postgres"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBName:      getEnv("DB_NAME", "pos_restaurant"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
		RunSeeder:   runSeeder,
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
