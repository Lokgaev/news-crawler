package main

import (
	"context"
	"log"

	"news-crawler/internal/config"
	"news-crawler/internal/repository/postgres"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("[CONFIG] ", err)
	}

	repo, err := postgres.New(ctx, cfg)
	if err != nil {
		log.Fatal("[POSTGRES] ", err)
	}
	defer repo.Close()

	log.Println("[POSTGRES] Подключение к базе данных успешно")

}
