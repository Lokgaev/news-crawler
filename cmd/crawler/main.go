package main

import (
	"context"
	"log"
	"time"

	"news-crawler/internal/config"
	"news-crawler/internal/repository"
	"news-crawler/internal/repository/postgres"
	"news-crawler/internal/scraper/orient"
	"news-crawler/internal/scraper/tdh"
	"news-crawler/internal/scraper/turkmenportal"
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

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		log.Println("[CRAWLER] Начинаем новый цикл парсинга")

		runCrawlers(ctx, repo, cfg)

		log.Println("[CRAWLER] Цикл завершён. Ожидаем следующий запуск")

		<-ticker.C
	}
}

func runCrawlers(
	ctx context.Context,
	repo repository.ArticleRepository,
	cfg config.Config,
) {
	log.Println("[TURKMENPORTAL] Начинаем парсинг")

	err := turkmenportal.Run(
		ctx,
		repo,
		cfg.MaxPages,
		cfg.MaxAgeDays,
	)

	if err != nil {
		log.Println("[TURKMENPORTAL] Ошибка:", err)
	} else {
		log.Println("[TURKMENPORTAL] Парсинг завершён")
	}

	log.Println("[ORIENT] Начинаем парсинг")

	err = orient.Run(
		ctx,
		repo,
		cfg.MaxPages,
		cfg.MaxAgeDays,
	)

	if err != nil {
		log.Println("[ORIENT] Ошибка:", err)
	} else {
		log.Println("[ORIENT] Парсинг завершён")
	}

	log.Println("[TDH] Начинаем парсинг")

	err = tdh.Run(
		ctx,
		repo,
		cfg.MaxPages,
		cfg.MaxAgeDays,
	)

	if err != nil {
		log.Println("[TDH] Ошибка:", err)
	} else {
		log.Println("[TDH] Парсинг завершён")
	}

	log.Println("[CRAWLER] Цикл парсинга завершён")
}
