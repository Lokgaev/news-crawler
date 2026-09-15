package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppAddr string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	MaxPages   int
	MaxAgeDays int
	MaxImageMB int64
}

func Load() (Config, error) {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("[CONFIG] .env файл	не найден, использую системные переменные окружения")
	}

	maxImageMB, err := strconv.ParseInt(
		os.Getenv("MAX_IMAGE_MB"),
		10,
		64,
	)
	if err != nil || maxImageMB <= 0 {
		return Config{}, fmt.Errorf(
			"неверный MAX_IMAGE_MB",
		)
	}

	maxPages, err := strconv.Atoi(os.Getenv("MAX_PAGES"))
	if err != nil {
		return Config{}, fmt.Errorf("неверный MAX_PAGES: %w", err)
	}

	maxAgeDays, err := strconv.Atoi(os.Getenv("MAX_AGE_DAYS"))
	if err != nil {
		return Config{}, fmt.Errorf("неверный MAX_AGE_DAYS: %w", err)
	}

	cfg := Config{
		AppAddr: os.Getenv("APP_ADDR"),

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:  os.Getenv("DB_SSLMODE"),

		MaxPages:   maxPages,
		MaxAgeDays: maxAgeDays,
		MaxImageMB: maxImageMB,
	}

	return cfg, nil
}
