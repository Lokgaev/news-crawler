package orient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"

	"news-crawler/internal/imagecheck"
	"news-crawler/internal/model"
	"news-crawler/internal/repository"
)

func Run(
	ctx context.Context,
	repo repository.ArticleRepository,
	maxPages int,
	maxAgeDays int,
	maxImageMB int64,
) error {

	if maxImageMB <= 0 {
		return fmt.Errorf("MAX_IMAGE_MB должен быть больше 0")
	}

	if maxPages <= 0 {
		return fmt.Errorf("MAX_PAGES должен быть больше 0")
	}

	if maxAgeDays <= 0 {
		return fmt.Errorf("MAX_AGE_DAYS должен быть больше 0")
	}

	languages := []string{
		"ru",
		"en",
		"tk",
	}

	categories := []string{
		"events",
		"economy",
		"society",
		"culture",
		"business",
		"sport",
		"science",
		"technology",
		"eco-world",
		"china",
		"partners",
	}

	cutoff := time.Now().AddDate(
		0,
		0,
		-maxAgeDays,
	)

	// Одна общая map для RU + EN + TK.

	fmt.Printf(
		"[ORIENT] Парсим максимум %d страниц на язык, новости не старше %d дней\n",
		maxPages,
		maxAgeDays,
	)

	fmt.Println(
		"[ORIENT] Минимальная дата:",
		cutoff.Format("2006-01-02"),
	)

	for _, category := range categories {
		seen := make(map[string]bool)
		for _, language := range languages {
			err := scrapeLanguage(
				ctx,
				repo,
				language,
				category,
				maxPages,
				cutoff,
				seen,
				maxImageMB,
			)
			if err != nil {
				return fmt.Errorf(
					"парсинг Orient категории %s, языка %s: %w",
					category,
					language,
					err,
				)
			}
		}

	}

	return nil
}

func scrapeLanguage(
	ctx context.Context,
	repo repository.ArticleRepository,
	language string,
	category string,
	maxPages int,
	cutoff time.Time,
	seen map[string]bool,
	maxImageMB int64,
) error {

	for page := 1; page <= maxPages; page++ {
		fmt.Printf(
			"[ORIENT][%s] ===== Страница %d =====\n",
			strings.ToUpper(language),
			page,
		)

		foundArticles, oldArticleFound, err := scrapeListPage(
			ctx,
			repo,
			language,
			category,
			page,
			seen,
			cutoff,
			maxImageMB,
		)
		if err != nil {
			return err
		}

		if foundArticles == 0 {
			fmt.Printf(
				"[ORIENT][%s] На странице %d нет новых статей. Останавливаюсь.\n",
				strings.ToUpper(language),
				page,
			)

			break
		}

		if oldArticleFound {
			fmt.Printf(
				"[ORIENT][%s] Найдены статьи старше допустимого срока. Дальше не идём.\n",
				strings.ToUpper(language),
			)

			break
		}
	}

	return nil
}

func scrapeListPage(
	ctx context.Context,
	repo repository.ArticleRepository,
	language string,
	category string,
	page int,
	seen map[string]bool,
	cutoff time.Time,
	maxImageMB int64,
) (int, bool, error) {
	listCollector := colly.NewCollector(
		colly.AllowURLRevisit(),
	)
	listCollector.SetRequestTimeout(30 * time.Second)

	var callbackErr error

	var oldArticleFound bool

	foundArticles := 0

	var listURL string

	if page == 1 {
		listURL = fmt.Sprintf(
			"https://orient.tm/%s/news/%s",
			language,
			category,
		)
	} else {
		listURL = fmt.Sprintf(
			"https://orient.tm/%s/news/%s?page=%d",
			language,
			category,
			page,
		)
	}

	articleSelector := fmt.Sprintf(
		`main div[class*="md:col-span-3"] > article > a[href*="/%s/posts/"]`,
		language,
	)

	listCollector.OnRequest(func(r *colly.Request) {
		fmt.Printf(
			"[ORIENT][%s] Запрос списка: %s\n",
			strings.ToUpper(language),
			r.URL,
		)
	})

	listCollector.OnHTML(
		articleSelector,
		func(e *colly.HTMLElement) {
			if callbackErr != nil {
				return
			}

			href := e.Attr("href")

			parts := strings.Split(
				strings.Trim(href, "/"),
				"/",
			)

			if len(parts) == 0 {
				return
			}

			externalID := parts[len(parts)-1]

			if externalID == "" {
				return
			}

			foundArticles++

			if seen[externalID] {
				return
			}

			exists, complete, hasCategory, err := repo.GetArticleStatus(
				ctx,
				"orient",
				externalID,
				category,
			)

			if err != nil {
				callbackErr = err
				return
			}

			if exists && complete && hasCategory {
				seen[externalID] = true

				fmt.Printf(
					"[ORIENT][%s] Статья %s уже полностью есть в БД, пропускаю\n",
					strings.ToUpper(language),
					externalID,
				)

				return
			}

			if exists && !complete {
				fmt.Printf(
					"[ORIENT][%s] Статья %s есть в БД, но не все переводы заполнены — проверяю снова\n",
					strings.ToUpper(language),
					externalID,
				)
			}

			articleURL := e.Request.AbsoluteURL(href)

			fmt.Printf(
				"[ORIENT][%s][%s] Найдена статья: %s\n",
				category,
				strings.ToUpper(language),
				articleURL,
			)

			article, isOld, err := scrapeArticle(
				articleURL,
				externalID,
				language,
				category,
				cutoff,
			)
			if err != nil {
				fmt.Printf(
					"[ORIENT][%s] Ошибка статьи %s: %v\n",
					strings.ToUpper(language),
					externalID,
					err,
				)

				return
			}

			if isOld {
				oldArticleFound = true
				seen[externalID] = true
				fmt.Printf(
					"[ORIENT][%s] Статья %s старше допустимого срока, пропускаю\n",
					strings.ToUpper(language),
					externalID,
				)

				return
			}

			article.ImgURL = imagecheck.FilterURL(
				ctx,
				article.ImgURL,
				maxImageMB,
			)

			err = repo.SaveArticle(ctx, article)
			if err != nil {
				callbackErr = err
				return
			}

			seen[externalID] = true

			fmt.Printf(
				"[ORIENT] Статья сохранена: %s\n",
				externalID,
			)
		},
	)

	err := visitWithRetry(
		listCollector,
		listURL,
		3,
	)
	if err != nil {
		return 0, false, fmt.Errorf(
			"открытие списка %s: %w",
			listURL,
			err,
		)
	}

	if callbackErr != nil {
		return 0, false, callbackErr
	}

	return foundArticles, oldArticleFound, nil
}

func scrapeArticle(
	articleURL string,
	externalID string,
	startLanguage string,
	category string,
	cutoff time.Time,
) (model.Article, bool, error) {
	article := model.Article{
		ExternalID: externalID,
		SourceName: "orient",
		ScrapedAt:  time.Now(),
		Published:  true,
		Category:   stringPointer(category),
	}

	alternateURLs := make(map[string]string)

	alternateURLs[startLanguage] = articleURL

	err := scrapeArticlePage(
		articleURL,
		startLanguage,
		&article,
		alternateURLs,
	)
	if err != nil {
		return model.Article{}, false, err
	}

	// Сначала проверяем возраст основной статьи.
	// Если она старая, нет смысла дополнительно
	// открывать её EN/TK/RU переводы.
	if article.PostedAt != nil {
		if article.PostedAt.Before(cutoff) {
			return article, true, nil
		}
	}

	languages := []string{
		"ru",
		"en",
		"tk",
	}

	for _, language := range languages {
		if language == startLanguage {
			continue
		}

		translatedURL, ok := alternateURLs[language]
		if !ok {
			continue
		}

		fmt.Printf(
			"[ORIENT][%s] Найден перевод: %s\n",
			strings.ToUpper(language),
			translatedURL,
		)

		err := scrapeArticlePage(
			translatedURL,
			language,
			&article,
			nil,
		)
		if err != nil {
			fmt.Printf(
				"[ORIENT][%s] Перевод пока недоступен: %v\n",
				strings.ToUpper(language),
				err,
			)

			continue
		}
	}

	return article, false, nil
}

func scrapeArticlePage(
	articleURL string,
	language string,
	article *model.Article,
	alternateURLs map[string]string,
) error {
	articleCollector := colly.NewCollector(
		colly.AllowURLRevisit(),
	)
	articleCollector.SetRequestTimeout(30 * time.Second)

	// Проверяем, что язык нам известен.
	switch language {
	case "ru", "en", "tk":
		// Всё нормально.

	default:
		return fmt.Errorf(
			"неподдерживаемый язык: %s",
			language,
		)
	}

	var parseErr error

	var title string
	var fullText string

	var postedAt *time.Time
	var imgURL *string

	var titleFound bool
	var textFound bool

	articleCollector.OnRequest(func(r *colly.Request) {
		fmt.Printf(
			"[ORIENT][%s] Открываю статью: %s\n",
			strings.ToUpper(language),
			r.URL,
		)
	})

	// Ищем ссылки на другие языковые версии.
	// Это делаем только для первой открытой версии статьи.
	if alternateURLs != nil {
		articleCollector.OnHTML(
			`link[rel="alternate"][hreflang]`,
			func(e *colly.HTMLElement) {
				hreflang := strings.ToLower(
					strings.TrimSpace(
						e.Attr("hreflang"),
					),
				)

				href := strings.TrimSpace(
					e.Attr("href"),
				)

				if href == "" {
					return
				}

				href = e.Request.AbsoluteURL(href)

				switch hreflang {
				case "ru":
					alternateURLs["ru"] = href

				case "en":
					alternateURLs["en"] = href

				case "tk", "tm":
					alternateURLs["tk"] = href

				case "x-default":
					if _, exists := alternateURLs["ru"]; !exists {
						alternateURLs["ru"] = href
					}
				}
			},
		)
	}

	// Заголовок.
	articleCollector.OnHTML(
		"h1",
		func(e *colly.HTMLElement) {
			if titleFound {
				return
			}

			title = strings.TrimSpace(e.Text)

			if title == "" {
				return
			}

			titleFound = true
		},
	)

	// Дата публикации.
	articleCollector.OnHTML(
		`meta[property="article:published_time"]`,
		func(e *colly.HTMLElement) {
			if postedAt != nil {
				return
			}

			rawDate := strings.TrimSpace(
				e.Attr("content"),
			)

			if rawDate == "" {
				return
			}

			parsedTime, err := time.Parse(
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

			postedAt = &parsedTime
		},
	)

	// Картинка.
	articleCollector.OnHTML(
		`meta[property="og:image"]`,
		func(e *colly.HTMLElement) {
			if imgURL != nil {
				return
			}

			value := strings.TrimSpace(
				e.Attr("content"),
			)

			if value == "" {
				return
			}

			imgURL = &value
		},
	)

	// Полный текст статьи.
	articleCollector.OnHTML(
		"article",
		func(e *colly.HTMLElement) {
			if textFound {
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

			fullText = strings.Join(
				cleanParagraphs,
				"\n\n",
			)

			textFound = true
		},
	)

	err := visitWithRetry(
		articleCollector,
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

	if parseErr != nil {
		return parseErr
	}

	if !titleFound {
		return fmt.Errorf(
			"не найден заголовок статьи %s",
			articleURL,
		)
	}

	if !textFound {
		return fmt.Errorf(
			"не найден текст статьи %s",
			articleURL,
		)
	}

	// Только после того, как страница успешно распарсилась,
	// записываем данные в model.Article.
	switch language {
	case "ru":
		article.TitleRU = stringPointer(title)
		article.TextRU = stringPointer(fullText)
		article.URLRU = stringPointer(articleURL)

	case "en":
		article.TitleEN = stringPointer(title)
		article.TextEN = stringPointer(fullText)
		article.URLEN = stringPointer(articleURL)

	case "tk":
		article.TitleTM = stringPointer(title)
		article.TextTM = stringPointer(fullText)
		article.URLTM = stringPointer(articleURL)
	}

	// Дату и картинку достаточно сохранить один раз.
	if article.PostedAt == nil && postedAt != nil {
		article.PostedAt = postedAt
	}

	if article.ImgURL == nil && imgURL != nil {
		article.ImgURL = imgURL
	}

	return nil
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
				"[ORIENT] Ошибка запроса. Повтор %d/%d: %v\n",
				attempt+1,
				maxAttempts,
				err,
			)

			time.Sleep(2 * time.Second)
		}
	}

	return lastErr
}
