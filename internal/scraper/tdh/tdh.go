package tdh

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"news-crawler/internal/model"
	"news-crawler/internal/repository"

	"github.com/gocolly/colly/v2"
)

func Run(
	ctx context.Context,
	repo repository.ArticleRepository,
	maxPages int,
	maxAgeDays int,
) error {

	if maxPages <= 0 {
		return fmt.Errorf("maxPages должен быть больше 0")
	}

	if maxAgeDays <= 0 {
		return fmt.Errorf("maxAgeDays должен быть больше 0")
	}

	now := time.Now().UTC()

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		time.UTC,
	)

	cutoff := today.AddDate(0, 0, -maxAgeDays)

	seen := make(map[string]bool)

	fmt.Println("[TDH] Собираю русские статьи для поиска переводов...")

	ruArticles, err := scrapeLanguagePages("ru", maxPages)
	if err != nil {
		return fmt.Errorf("сбор русских статей: %w", err)
	}

	fmt.Println("[TDH] Собираю туркменские статьи для поиска переводов...")

	tkArticles, err := scrapeLanguagePages("tk", maxPages)
	if err != nil {
		return fmt.Errorf("сбор туркменских статей: %w", err)
	}

	fmt.Println("[TDH] Собираю английские статьи для поиска переводов...")

	enArticles, err := scrapeLanguagePages("en", maxPages)
	if err != nil {
		return fmt.Errorf("сбор английских статей: %w", err)
	}

	ruImageIndex := buildArticleIndex(ruArticles)
	tkImageIndex := buildArticleIndex(tkArticles)
	enImageIndex := buildArticleIndex(enArticles)

	fmt.Printf(
		"[TDH] Парсим максимум %d страниц, статьи не старше %d дней\n",
		maxPages,
		maxAgeDays,
	)

	fmt.Printf(
		"[TDH] Минимальная дата: %s\n",
		cutoff.Format("2006-01-02"),
	)

	for page := 1; page <= maxPages; page++ {

		fmt.Printf(
			"[TDH] ===== Страница %d =====\n",
			page,
		)

		listURL := "https://tdh.gov.tm/ru/category/default"

		if page > 1 {
			listURL = fmt.Sprintf(
				"https://tdh.gov.tm/ru/category/default?page=%d",
				page,
			)
		}

		fmt.Println("[TDH] Открываю список:", listURL)

		newArticles := 0
		var callbackErr error

		c := colly.NewCollector(
			colly.AllowURLRevisit(),
		)

		c.SetRequestTimeout(30 * time.Second)

		c.OnHTML(".main_news-title a", func(e *colly.HTMLElement) {

			if callbackErr != nil {
				return
			}

			articleURL := strings.TrimSpace(
				e.Attr("href"),
			)

			if articleURL == "" {
				return
			}

			externalID, err := extractExternalID(articleURL)
			if err != nil {
				callbackErr = err
				return
			}

			if seen[externalID] {
				return
			}

			seen[externalID] = true

			fmt.Println(
				"[TDH] Найдена статья:",
				articleURL,
			)

			article := model.Article{
				ExternalID: externalID,
				SourceName: "tdh",
				ScrapedAt:  time.Now(),
				Published:  true,
				URLRU:      stringPointer(articleURL),
			}

			err = scrapeArticlePage(
				articleURL,
				&article,
			)

			if err != nil {
				callbackErr = err
				return
			}

			if article.PostedAt == nil {
				fmt.Println("[TDH] У статьи нет даты, пропускаю:", externalID)
				return
			}

			articleDate := article.PostedAt.UTC()
			cutoffDate := cutoff.UTC()

			if articleDate.Before(cutoffDate) {
				fmt.Printf(
					"[TDH] Статья %s слишком старая (%s), пропускаю\n",
					externalID,
					articleDate.Format("2006-01-02"),
				)
				return
			}

			ruMatch := listArticle{
				URL:      articleURL,
				PostedAt: article.PostedAt.Format("02.01.2006"),
			}

			if article.ImgURL != nil {
				ruMatch.ImageURL = *article.ImgURL
			}

			if article.Category != nil {
				ruMatch.Category = *article.Category
			}

			tkArticle, tkFound := findTranslationMatch(
				ruMatch,
				ruImageIndex,
				tkImageIndex,
			)

			if tkFound {
				fmt.Println("[TDH] Найден TK перевод [image]:", tkArticle.URL)

				translationErr := scrapeTranslationPage(tkArticle.URL, "tk", &article)
				if translationErr != nil {
					fmt.Println("[TDH] Не удалось получить TK перевод:", translationErr)
				}
			}

			enArticle, enFound := findTranslationMatch(
				ruMatch,
				ruImageIndex,
				enImageIndex,
			)

			if enFound {
				fmt.Println("[TDH] Найден EN перевод [image]:", enArticle.URL)

				translationErr := scrapeTranslationPage(enArticle.URL, "en", &article)
				if translationErr != nil {
					fmt.Println("[TDH] Не удалось получить EN перевод:", translationErr)
				}
			}

			err = repo.SaveArticle(
				ctx,
				article,
			)

			if err != nil {
				callbackErr = fmt.Errorf(
					"сохранение статьи %s: %w",
					externalID,
					err,
				)
				return
			}

			newArticles++

			fmt.Println(
				"[TDH] Статья сохранена:",
				externalID,
			)
		})

		err := visitWithRetry(
			c,
			listURL,
			3,
		)

		if err != nil {
			return fmt.Errorf(
				"открытие списка %s: %w",
				listURL,
				err,
			)
		}

		if callbackErr != nil {
			return callbackErr
		}

		if newArticles == 0 {
			fmt.Println(
				"[TDH] Новых статей нет. Дальше не идём.",
			)
			break
		}
	}

	return nil
}

func scrapeTranslationPage(
	articleURL string,
	language string,
	article *model.Article,
) error {

	c := colly.NewCollector(
		colly.AllowURLRevisit(),
	)

	c.SetRequestTimeout(30 * time.Second)

	var title string
	var textParts []string

	c.OnHTML(".news_page-title", func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Text)
	})

	c.OnHTML(".news_page-text p", func(e *colly.HTMLElement) {
		if strings.Contains(e.Attr("class"), "para_text") {
			return
		}

		text := strings.TrimSpace(e.Text)

		if text == "" {
			return
		}

		textParts = append(textParts, text)
	})

	err := visitWithRetry(c, articleURL, 3)
	if err != nil {
		return err
	}

	text := strings.Join(textParts, "\n\n")

	if title == "" {
		return fmt.Errorf("не найден заголовок перевода: %s", articleURL)
	}

	if text == "" {
		return fmt.Errorf("не найден текст перевода: %s", articleURL)
	}

	switch language {
	case "tk":
		article.TitleTM = stringPointer(title)
		article.TextTM = stringPointer(text)
		article.URLTM = stringPointer(articleURL)

	case "en":
		article.TitleEN = stringPointer(title)
		article.TextEN = stringPointer(text)
		article.URLEN = stringPointer(articleURL)

	default:
		return fmt.Errorf("неизвестный язык: %s", language)
	}

	return nil
}

func scrapeArticlePage(
	articleURL string,
	article *model.Article,
) error {

	c := colly.NewCollector(
		colly.AllowURLRevisit(),
	)

	c.SetRequestTimeout(30 * time.Second)

	var title string
	var textParts []string
	var postedAt time.Time
	var category string
	var imageURL string

	c.OnHTML(".news_page-title", func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Text)
	})

	c.OnHTML(".single_slider img, .multiple_photo img, .multiple_item img", func(e *colly.HTMLElement) {
		if imageURL != "" {
			return
		}

		imageURL = strings.TrimSpace(e.Attr("src"))
	})

	c.OnHTML(".news_page-text p", func(e *colly.HTMLElement) {

		if strings.Contains(
			e.Attr("class"),
			"para_text",
		) {
			return
		}

		text := strings.TrimSpace(e.Text)

		if text == "" {
			return
		}

		textParts = append(
			textParts,
			text,
		)
	})

	c.OnHTML(".publish_date", func(e *colly.HTMLElement) {

		values := e.ChildTexts("span")

		if len(values) < 2 {
			return
		}

		rawDate := strings.TrimSpace(
			values[len(values)-1],
		)

		value, err := time.Parse(
			"02.01.2006",
			rawDate,
		)

		if err != nil {
			return
		}

		postedAt = value
	})

	c.OnHTML(
		`a.page_name[href*="/category/"]`,
		func(e *colly.HTMLElement) {

			href := strings.TrimSpace(
				e.Attr("href"),
			)

			parts := strings.Split(
				href,
				"/category/",
			)

			if len(parts) != 2 {
				return
			}

			category = normalizeCategory(
				strings.Trim(
					parts[1],
					"/",
				),
			)
		},
	)

	err := visitWithRetry(
		c,
		articleURL,
		3,
	)

	if err != nil {
		return fmt.Errorf(
			"открытие статьи %s: %w",
			articleURL,
			err,
		)
	}

	if title == "" {
		return fmt.Errorf(
			"не найден заголовок: %s",
			articleURL,
		)
	}

	text := strings.Join(
		textParts,
		"\n\n",
	)

	if text == "" {
		return fmt.Errorf(
			"не найден текст: %s",
			articleURL,
		)
	}

	if postedAt.IsZero() {
		return fmt.Errorf(
			"не найдена дата: %s",
			articleURL,
		)
	}

	article.TitleRU = stringPointer(title)
	article.TextRU = stringPointer(text)
	article.PostedAt = &postedAt
	article.URLRU = stringPointer(articleURL)

	if category != "" {
		article.Category = stringPointer(category)
	}

	if imageURL != "" {
		article.ImgURL = stringPointer(imageURL)
	}

	return nil
}

func extractExternalID(articleURL string) (string, error) {
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return "", err
	}

	parts := strings.Split(
		strings.Trim(parsedURL.Path, "/"),
		"/",
	)

	if len(parts) < 4 {
		return "", fmt.Errorf("неожиданный URL: %s", articleURL)
	}

	if parts[1] != "post" {
		return "", fmt.Errorf("это не URL статьи: %s", articleURL)
	}

	return parts[2], nil
}

func normalizeCategory(category string) string {
	categories := map[string]string{
		"politika":          "politics",
		"syyasat-habarlary": "politics",

		"ekonomika":   "economy",
		"ykdysadyyet": "economy",

		"obshchestvo": "society",

		"kultura":   "culture",
		"medeniyet": "culture",

		"sport":       "sport",
		"kommentarii": "comments",

		"hronika":  "chronicle",
		"resmi":    "chronicle",
		"official": "chronicle",

		"sobytiya-nedeli": "events",
	}

	value, ok := categories[category]
	if ok {
		return value
	}

	return category
}

type listArticle struct {
	Title    string
	URL      string
	PostedAt string
	ImageURL string
	Category string
}

func stringPointer(value string) *string {
	return &value
}

func visitWithRetry(
	c *colly.Collector,
	url string,
	maxAttempts int,
) error {

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {

		err := c.Visit(url)

		if err == nil {
			return nil
		}

		lastErr = err

		if attempt < maxAttempts {
			fmt.Printf(
				"[TDH] Ошибка запроса. Повтор %d/%d: %v\n",
				attempt+1,
				maxAttempts,
				err,
			)

			time.Sleep(2 * time.Second)
		}
	}

	return lastErr
}

func scrapeListArticleDetails(articleURL string) (string, string, string, error) {
	c := colly.NewCollector(colly.AllowURLRevisit())
	c.SetRequestTimeout(30 * time.Second)

	var imageURL string
	var postedAt string
	var category string

	c.OnHTML(".single_slider img, .multiple_photo img, .multiple_item img", func(e *colly.HTMLElement) {
		if imageURL != "" {
			return
		}

		imageURL = strings.TrimSpace(e.Attr("src"))
	})

	c.OnHTML(`a.page_name[href*="/category/"]`, func(e *colly.HTMLElement) {
		href := strings.TrimSpace(e.Attr("href"))
		parts := strings.Split(href, "/category/")

		if len(parts) != 2 {
			return
		}

		category = normalizeCategory(strings.Trim(parts[1], "/"))
	})

	c.OnHTML(".publish_date", func(e *colly.HTMLElement) {
		values := e.ChildTexts("span")

		if len(values) < 2 {
			return
		}

		postedAt = strings.TrimSpace(values[len(values)-1])
	})

	err := visitWithRetry(c, articleURL, 3)
	if err != nil {
		return "", "", "", err
	}

	return imageURL, postedAt, category, nil
}

func buildMatchKey(article listArticle) string {
	if article.PostedAt == "" || article.ImageURL == "" {
		return ""
	}

	return article.PostedAt + "|" + article.ImageURL
}

func buildArticleIndex(articles []listArticle) map[string][]listArticle {
	index := make(map[string][]listArticle)

	for _, article := range articles {
		key := buildMatchKey(article)

		if key == "" {
			continue
		}

		index[key] = append(index[key], article)
	}

	return index
}

func scrapeLanguagePages(language string, maxPages int) ([]listArticle, error) {
	var articles []listArticle

	for page := 1; page <= maxPages; page++ {
		listURL := fmt.Sprintf("https://tdh.gov.tm/%s/category/default", language)

		if page > 1 {
			listURL = fmt.Sprintf("https://tdh.gov.tm/%s/category/default?page=%d", language, page)
		}

		c := colly.NewCollector()
		var callbackErr error

		c.OnHTML(".main_news-title a", func(e *colly.HTMLElement) {
			if callbackErr != nil {
				return
			}

			title := strings.TrimSpace(e.Text)
			articleURL := strings.TrimSpace(e.Attr("href"))

			if title == "" || articleURL == "" {
				return
			}

			imageURL, postedAt, category, err := scrapeListArticleDetails(articleURL)
			if err != nil {
				callbackErr = err
				return
			}

			articles = append(articles, listArticle{
				Title:    title,
				URL:      articleURL,
				PostedAt: postedAt,
				ImageURL: imageURL,
				Category: category,
			})
		})

		err := visitWithRetry(c, listURL, 3)
		if err != nil {
			return nil, err
		}

		if callbackErr != nil {
			return nil, callbackErr
		}
	}

	return articles, nil
}

func findTranslationMatch(
	article listArticle,
	sourceImageIndex map[string][]listArticle,
	targetImageIndex map[string][]listArticle,
) (listArticle, bool) {

	key := buildMatchKey(article)

	if key == "" {
		return listArticle{}, false
	}

	sourceCandidates := sourceImageIndex[key]
	targetCandidates := targetImageIndex[key]

	if len(sourceCandidates) != 1 || len(targetCandidates) != 1 {
		return listArticle{}, false
	}

	return targetCandidates[0], true
}
