package orient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"

	"news-crawler/internal/model"
	"news-crawler/internal/repository"
)

func Run(
	ctx context.Context,
	repo repository.ArticleRepository,
) error {
	listCollector := colly.NewCollector()

	listCollector.OnRequest(func(r *colly.Request) {
		fmt.Println("[ORIENT] Запрос списка:", r.URL)
	})

	listCollector.OnHTML(
		`a[href*="/ru/posts/"]`,
		func(e *colly.HTMLElement) {
			href := e.Attr("href")

			parts := strings.Split(href, "/")
			externalID := parts[len(parts)-1]

			articleURL := e.Request.AbsoluteURL(href)

			fmt.Println("[ORIENT] Найдена статья:", articleURL)
			fmt.Println("[ORIENT] ExternalID:", externalID)

			article, err := scrapeArticle(
				articleURL,
				externalID,
			)
			if err != nil {
				fmt.Println("[ORIENT] Ошибка парсинга:", err)
				return
			}

			err = repo.SaveArticle(ctx, article)
			if err != nil {
				fmt.Println("[ORIENT] Ошибка сохранения:", err)
				return
			}

			fmt.Println(
				"[ORIENT] Статья сохранена:",
				externalID,
			)
		},
	)

	err := listCollector.Visit("https://orient.tm/ru/news")
	if err != nil {
		return fmt.Errorf(
			"открытие списка Orient: %w",
			err,
		)
	}

	return nil
}

func scrapeArticle(
	articleURL string,
	externalID string,
) (model.Article, error) {
	articleCollector := colly.NewCollector()

	article := model.Article{
		ExternalID: externalID,
		SourceName: "orient",
		ScrapedAt:  time.Now(),
		URLRU:      stringPointer(articleURL),
		Published:  true,
	}

	var parseErr error

	articleCollector.OnRequest(func(r *colly.Request) {
		fmt.Println("[ORIENT] Открываю статью:", r.URL)
	})

	articleCollector.OnHTML(
		"h1",
		func(e *colly.HTMLElement) {
			title := strings.TrimSpace(e.Text)

			if title == "" {
				return
			}

			article.TitleRU = &title
		},
	)

	articleCollector.OnHTML(
		`meta[property="article:published_time"]`,
		func(e *colly.HTMLElement) {
			rawDate := e.Attr("content")

			postedAt, err := time.Parse(
				time.RFC3339Nano,
				rawDate,
			)
			if err != nil {
				parseErr = fmt.Errorf(
					"разбор даты %q: %w",
					rawDate,
					err,
				)
				return
			}

			article.PostedAt = &postedAt
		},
	)

	articleCollector.OnHTML(
		`meta[property="og:image"]`,
		func(e *colly.HTMLElement) {
			imgURL := strings.TrimSpace(
				e.Attr("content"),
			)

			if imgURL == "" {
				return
			}

			article.ImgURL = &imgURL
		},
	)

	articleCollector.OnHTML(
		"article",
		func(e *colly.HTMLElement) {
			if article.TextRU != nil {
				return
			}

			paragraphs := e.ChildTexts("p")

			if len(paragraphs) == 0 {
				return
			}

			var cleanParagraphs []string

			for _, paragraph := range paragraphs {
				paragraph = strings.TrimSpace(paragraph)

				if paragraph == "" {
					continue
				}

				if paragraph == "ORIENT" {
					continue
				}

				cleanParagraphs = append(
					cleanParagraphs,
					paragraph,
				)
			}

			if len(cleanParagraphs) == 0 {
				return
			}

			fullText := strings.Join(
				cleanParagraphs,
				"\n\n",
			)

			article.TextRU = &fullText
		},
	)

	err := articleCollector.Visit(articleURL)
	if err != nil {
		return model.Article{}, fmt.Errorf(
			"открытие статьи %s: %w",
			articleURL,
			err,
		)
	}

	if parseErr != nil {
		return model.Article{}, parseErr
	}

	if article.TitleRU == nil {
		return model.Article{}, fmt.Errorf(
			"не найден заголовок статьи %s",
			externalID,
		)
	}

	if article.TextRU == nil {
		return model.Article{}, fmt.Errorf(
			"не найден текст статьи %s",
			externalID,
		)
	}

	return article, nil
}

func stringPointer(value string) *string {
	return &value
}
