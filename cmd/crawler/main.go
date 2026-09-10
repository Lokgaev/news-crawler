package main

import (
	"context"
	"log"

	"news-crawler/internal/config"
	"news-crawler/internal/repository/postgres"

	"news-crawler/internal/scraper/orient"
	/* "news-crawler/internal/scraper/turkmenportal" */)

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

	/* err = turkmenportal.Run(ctx, repo, cfg.MaxAgeDays, cfg.MaxPages)
	if err != nil {
		fmt.Println("Ошибка Turkmenportal:", err)
		return
	} */

	err = orient.Run(ctx, repo, cfg.MaxPages, cfg.MaxAgeDays)
	if err != nil {
		log.Fatal("[ORIENT] ", err)
	}
}
