package repository

import (
	"context"

	"news-crawler/internal/model"
)

type ArticleRepository interface {
	SaveArticle(ctx context.Context, article model.Article) error
}
