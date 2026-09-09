package postgres

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"

	"news-crawler/internal/config"
	"news-crawler/internal/model"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg config.Config) (*Repository, error) {
	dsnURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:   cfg.DBHost + ":" + cfg.DBPort,
		Path:   "/" + cfg.DBName,
	}

	query := dsnURL.Query()
	query.Set("sslmode", cfg.DBSSLMode)
	dsnURL.RawQuery = query.Encode()

	dsn := dsnURL.String()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("создание пула PostgreSQL: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("проверка подключения PostgreSQL: %w", err)
	}

	return &Repository{
		pool: pool,
	}, nil

}

func (r *Repository) SaveArticle(
	ctx context.Context,
	article model.Article,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"начало транзакции: %w",
			err,
		)
	}

	defer tx.Rollback(ctx)

	const articleQuery = `
		INSERT INTO articles (
			external_id,
			source_name,

			title_tm,
			title_ru,
			title_en,

			text_tm,
			text_ru,
			text_en,

			posted_at,
			scraped_at,
			img_url,

			url_tm,
			url_ru,
			url_en,

			published
		)
		VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14,
			$15
		)

		ON CONFLICT (source_name, external_id)
		DO UPDATE SET
			title_tm = COALESCE(
				EXCLUDED.title_tm,
				articles.title_tm
			),

			title_ru = COALESCE(
				EXCLUDED.title_ru,
				articles.title_ru
			),

			title_en = COALESCE(
				EXCLUDED.title_en,
				articles.title_en
			),

			text_tm = COALESCE(
				EXCLUDED.text_tm,
				articles.text_tm
			),

			text_ru = COALESCE(
				EXCLUDED.text_ru,
				articles.text_ru
			),

			text_en = COALESCE(
				EXCLUDED.text_en,
				articles.text_en
			),

			posted_at = COALESCE(
				EXCLUDED.posted_at,
				articles.posted_at
			),

			scraped_at = EXCLUDED.scraped_at,

			img_url = COALESCE(
				EXCLUDED.img_url,
				articles.img_url
			),

			url_tm = COALESCE(
				EXCLUDED.url_tm,
				articles.url_tm
			),

			url_ru = COALESCE(
				EXCLUDED.url_ru,
				articles.url_ru
			),

			url_en = COALESCE(
				EXCLUDED.url_en,
				articles.url_en
			),

			published = EXCLUDED.published

		RETURNING id
	`

	var articleID int64

	err = tx.QueryRow(
		ctx,
		articleQuery,

		article.ExternalID,
		article.SourceName,

		article.TitleTM,
		article.TitleRU,
		article.TitleEN,

		article.TextTM,
		article.TextRU,
		article.TextEN,

		article.PostedAt,
		article.ScrapedAt,
		article.ImgURL,

		article.URLTM,
		article.URLRU,
		article.URLEN,

		article.Published,
	).Scan(&articleID)

	if err != nil {
		return fmt.Errorf(
			"сохранение статьи %s/%s: %w",
			article.SourceName,
			article.ExternalID,
			err,
		)
	}

	if article.Category != nil &&
		*article.Category != "" {

		const categoryQuery = `
			INSERT INTO categories (slug)
			VALUES ($1)

			ON CONFLICT (slug)
			DO UPDATE SET
				slug = EXCLUDED.slug

			RETURNING id
		`

		var categoryID int64

		err = tx.QueryRow(
			ctx,
			categoryQuery,
			*article.Category,
		).Scan(&categoryID)

		if err != nil {
			return fmt.Errorf(
				"сохранение категории %s: %w",
				*article.Category,
				err,
			)
		}

		const relationQuery = `
			INSERT INTO article_categories (
				article_id,
				category_id
			)
			VALUES ($1, $2)

			ON CONFLICT DO NOTHING
		`

		_, err = tx.Exec(
			ctx,
			relationQuery,
			articleID,
			categoryID,
		)

		if err != nil {
			return fmt.Errorf(
				"связь статьи %s/%s с категорией %s: %w",
				article.SourceName,
				article.ExternalID,
				*article.Category,
				err,
			)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf(
			"завершение транзакции: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) Close() {
	r.pool.Close()
}
