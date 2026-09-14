package main

import (
	"context"
	"log"
	"net/http"

	"news-crawler/internal/config"
	"news-crawler/internal/httpapi"
	"news-crawler/internal/repository/postgres"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("[CONFIG] ", err)
	}

	repo, err := postgres.New(
		ctx,
		cfg,
	)
	if err != nil {
		log.Fatal("[POSTGRES] ", err)
	}

	defer repo.Close()

	log.Println(
		"[POSTGRES] Подключение к базе данных успешно",
	)

	api := httpapi.New(repo)

	log.Println(
		"[API] Сервер запущен на",
		cfg.AppAddr,
	)

	err = http.ListenAndServe(
		cfg.AppAddr,
		api.Handler(),
	)

	if err != nil {
		log.Fatal("[API] ", err)
	}
}
