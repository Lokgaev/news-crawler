package repository

import (
	"context"

	"news-crawler/internal/model"
)

type ArticleRepository interface {
	SaveArticle(ctx context.Context, article model.Article) error

	GetArticleStatus(
		ctx context.Context,
		sourceName string,
		externalID string,
		category string,
	) (exists bool, complete bool, hasCategory bool, err error)

	GetArticleByID(
		ctx context.Context,
		id int64,
	) (model.Article, bool, error)

	ListArticles(
		ctx context.Context,
		limit int,
		offset int,
		source string,
		category string,
		language string,
	) ([]model.Article, int, error)

	ListCategories(
		ctx context.Context,
	) ([]string, error)
}
