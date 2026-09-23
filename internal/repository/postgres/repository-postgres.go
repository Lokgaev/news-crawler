package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"

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

func (r *Repository) GetArticleStatus(
	ctx context.Context,
	sourceName string,
	externalID string,
	category string,
) (bool, bool, bool, error) {

	const query = `
		SELECT
			(
				a.title_tm IS NOT NULL
				AND a.text_tm IS NOT NULL
				AND a.url_tm IS NOT NULL

				AND a.title_ru IS NOT NULL
				AND a.text_ru IS NOT NULL
				AND a.url_ru IS NOT NULL

				AND a.title_en IS NOT NULL
				AND a.text_en IS NOT NULL
				AND a.url_en IS NOT NULL
			) AS complete,

			EXISTS (
				SELECT 1
				FROM article_categories ac
				JOIN categories c
					ON c.id = ac.category_id
				WHERE ac.article_id = a.id
					AND c.slug = $3
			) AS has_category

		FROM articles a
		WHERE a.source_name = $1
			AND a.external_id = $2
	`

	var complete bool
	var hasCategory bool

	err := r.pool.QueryRow(
		ctx,
		query,
		sourceName,
		externalID,
		category,
	).Scan(
		&complete,
		&hasCategory,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, false, false, nil
		}

		return false, false, false, fmt.Errorf(
			"проверка статьи %s/%s: %w",
			sourceName,
			externalID,
			err,
		)
	}

	return true, complete, hasCategory, nil
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

func (r *Repository) ListArticles(
	ctx context.Context,
	limit int,
	offset int,
	source string,
	category string,
	language string,
) ([]model.Article, int, error) {

	const countQuery = `
	WITH filtered AS (
		SELECT a.*
		FROM articles a
		WHERE a.published = TRUE

			AND (
				$1 = ''
				OR a.source_name = $1
			)

			AND (
				$2 = ''
				OR EXISTS (
					SELECT 1
					FROM article_categories ac
					JOIN categories c
						ON c.id = ac.category_id
					WHERE ac.article_id = a.id
						AND c.slug = $2
				)
			)

			AND (
				$3 = ''

				OR (
					$3 = 'ru'
					AND a.title_ru IS NOT NULL
					AND a.text_ru IS NOT NULL
				)

				OR (
					$3 = 'en'
					AND a.title_en IS NOT NULL
					AND a.text_en IS NOT NULL
				)

				OR (
					$3 = 'tm'
					AND a.title_tm IS NOT NULL
					AND a.text_tm IS NOT NULL
				)
			)
	),

	deduplicated AS (
		SELECT
			f.*,

			ROW_NUMBER() OVER (
				PARTITION BY
					f.source_name,
					COALESCE(
						f.title_ru,
						f.title_en,
						f.title_tm,
						''
					),
					COALESCE(
						f.text_ru,
						f.text_en,
						f.text_tm,
						''
					),
					COALESCE(
						f.img_url,
						''
					)

				ORDER BY
					(
						CASE
							WHEN f.title_ru IS NOT NULL
								AND f.text_ru IS NOT NULL
								AND f.url_ru IS NOT NULL
							THEN 1
							ELSE 0
						END

						+

						CASE
							WHEN f.title_en IS NOT NULL
								AND f.text_en IS NOT NULL
								AND f.url_en IS NOT NULL
							THEN 1
							ELSE 0
						END

						+

						CASE
							WHEN f.title_tm IS NOT NULL
								AND f.text_tm IS NOT NULL
								AND f.url_tm IS NOT NULL
							THEN 1
							ELSE 0
						END
					) DESC,

					f.posted_at DESC NULLS LAST,
					f.id DESC
			) AS rn

		FROM filtered f
	)

	SELECT COUNT(*)
	FROM deduplicated
	WHERE rn = 1
`

	var total int

	err := r.pool.QueryRow(
		ctx,
		countQuery,
		source,
		category,
		language,
	).Scan(&total)

	if err != nil {
		return nil, 0, fmt.Errorf(
			"подсчёт статей: %w",
			err,
		)
	}

	const articlesQuery = `
	WITH filtered AS (
		SELECT a.*
		FROM articles a
		WHERE a.published = TRUE

			AND (
				$1 = ''
				OR a.source_name = $1
			)

			AND (
				$2 = ''
				OR EXISTS (
					SELECT 1
					FROM article_categories ac
					JOIN categories c
						ON c.id = ac.category_id
					WHERE ac.article_id = a.id
						AND c.slug = $2
				)
			)

			AND (
				$3 = ''

				OR (
					$3 = 'ru'
					AND a.title_ru IS NOT NULL
					AND a.text_ru IS NOT NULL
				)

				OR (
					$3 = 'en'
					AND a.title_en IS NOT NULL
					AND a.text_en IS NOT NULL
				)

				OR (
					$3 = 'tm'
					AND a.title_tm IS NOT NULL
					AND a.text_tm IS NOT NULL
				)
			)
	),

	deduplicated AS (
		SELECT
			f.*,

			ROW_NUMBER() OVER (
				PARTITION BY
					f.source_name,
					COALESCE(
						f.title_ru,
						f.title_en,
						f.title_tm,
						''
					),
					COALESCE(
						f.text_ru,
						f.text_en,
						f.text_tm,
						''
					),
					COALESCE(
						f.img_url,
						''
					)

				ORDER BY
					(
						CASE
							WHEN f.title_ru IS NOT NULL
								AND f.text_ru IS NOT NULL
								AND f.url_ru IS NOT NULL
							THEN 1
							ELSE 0
						END

						+

						CASE
							WHEN f.title_en IS NOT NULL
								AND f.text_en IS NOT NULL
								AND f.url_en IS NOT NULL
							THEN 1
							ELSE 0
						END

						+

						CASE
							WHEN f.title_tm IS NOT NULL
								AND f.text_tm IS NOT NULL
								AND f.url_tm IS NOT NULL
							THEN 1
							ELSE 0
						END
					) DESC,

					f.posted_at DESC NULLS LAST,
					f.id DESC
			) AS rn

		FROM filtered f
	)

	SELECT
		a.id,
		a.external_id,

		a.title_tm,
		a.title_ru,
		a.title_en,

		CASE
			WHEN a.text_tm IS NULL THEN NULL
			WHEN char_length(a.text_tm) <= 100 THEN a.text_tm
			ELSE LEFT(a.text_tm, 97) || '...'
		END AS text_tm,

		CASE
			WHEN a.text_ru IS NULL THEN NULL
			WHEN char_length(a.text_ru) <= 100 THEN a.text_ru
			ELSE LEFT(a.text_ru, 97) || '...'
		END AS text_ru,

		CASE
			WHEN a.text_en IS NULL THEN NULL
			WHEN char_length(a.text_en) <= 100 THEN a.text_en
			ELSE LEFT(a.text_en, 97) || '...'
		END AS text_en,

		a.posted_at,
		a.scraped_at,
		a.img_url,

		a.url_tm,
		a.url_ru,
		a.url_en,

		a.source_name,
		a.published,

		ARRAY(
        	SELECT c.slug
			FROM article_categories ac
			JOIN categories c
                ON c.id = ac.category_id
			WHERE ac.article_id = a.id
			ORDER BY c.slug
	) AS categories

	FROM deduplicated a

	WHERE a.rn = 1

	ORDER BY
		a.posted_at DESC NULLS LAST,
		a.id DESC

	LIMIT $4
	OFFSET $5
`

	rows, err := r.pool.Query(
		ctx,
		articlesQuery,
		source,
		category,
		language,
		limit,
		offset,
	)

	if err != nil {
		return nil, 0, fmt.Errorf(
			"получение статей: %w",
			err,
		)
	}

	defer rows.Close()

	articles := make([]model.Article, 0)

	for rows.Next() {
		var article model.Article

		err := rows.Scan(
			&article.ID,
			&article.ExternalID,

			&article.TitleTM,
			&article.TitleRU,
			&article.TitleEN,

			&article.TextTM,
			&article.TextRU,
			&article.TextEN,

			&article.PostedAt,
			&article.ScrapedAt,
			&article.ImgURL,

			&article.URLTM,
			&article.URLRU,
			&article.URLEN,

			&article.SourceName,
			&article.Published,
			&article.Categories,
		)

		if err != nil {
			return nil, 0, fmt.Errorf(
				"чтение статьи из PostgreSQL: %w",
				err,
			)
		}

		articles = append(
			articles,
			article,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf(
			"чтение списка статей: %w",
			err,
		)
	}

	return articles, total, nil
}

func (r *Repository) GetArticleByID(
	ctx context.Context,
	id int64,
) (model.Article, bool, error) {
	const query = `
		SELECT
			a.id,
			a.external_id,

			a.title_tm,
			a.title_ru,
			a.title_en,

			a.text_tm,
			a.text_ru,
			a.text_en,

			a.posted_at,
			a.scraped_at,
			a.img_url,

			a.url_tm,
			a.url_ru,
			a.url_en,

			a.source_name,
			a.published,

			ARRAY(
				SELECT c.slug
				FROM article_categories ac
				JOIN categories c
                	ON c.id = ac.category_id
				WHERE ac.article_id = a.id
				ORDER BY c.slug
		) AS categories

		FROM articles a
		WHERE a.id = $1
			AND a.published = TRUE
	`

	var article model.Article

	err := r.pool.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&article.ID,
		&article.ExternalID,

		&article.TitleTM,
		&article.TitleRU,
		&article.TitleEN,

		&article.TextTM,
		&article.TextRU,
		&article.TextEN,

		&article.PostedAt,
		&article.ScrapedAt,
		&article.ImgURL,

		&article.URLTM,
		&article.URLRU,
		&article.URLEN,

		&article.SourceName,
		&article.Published,
		&article.Categories,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.Article{}, false, nil
	}

	if err != nil {
		return model.Article{}, false, fmt.Errorf(
			"получение статьи %d: %w",
			id,
			err,
		)
	}

	return article, true, nil
}

func (r *Repository) ListCategories(
	ctx context.Context,
) ([]string, error) {

	const query = `
		SELECT slug
		FROM categories
		ORDER BY slug
	`

	rows, err := r.pool.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"получение категорий: %w",
			err,
		)
	}

	defer rows.Close()

	categories := make([]string, 0)

	for rows.Next() {
		var category string

		err := rows.Scan(&category)
		if err != nil {
			return nil, fmt.Errorf(
				"чтение категории: %w",
				err,
			)
		}

		categories = append(
			categories,
			category,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"чтение списка категорий: %w",
			err,
		)
	}

	return categories, nil
}

func (r *Repository) Close() {
	r.pool.Close()
}
