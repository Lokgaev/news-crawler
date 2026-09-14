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

	ListArticles(
		ctx context.Context,
		limit int,
		offset int,
		source string,
		category string,
		language string,
	) ([]model.Article, int, error)
}
