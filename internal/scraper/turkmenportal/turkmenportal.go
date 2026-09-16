package turkmenportal

import (
	"context"

	"fmt"

	"news-crawler/internal/model"
	"news-crawler/internal/repository"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

func Run(
	ctx context.Context,
	repo repository.ArticleRepository,
	maxPages int,
	maxAgeDays int,

) error {

	if maxPages <= 0 {
		return fmt.Errorf("MAX_PAGES должен быть больше 0")
	}

	if maxAgeDays <= 0 {
		return fmt.Errorf("MAX_AGE_DAYS должен быть больше 0")
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	seen := make(map[string]bool)

	fmt.Printf(
		"[TURKMENPORTAL] Парсим максимум %d страниц, статьи не старше %d дней\n",
		maxPages,
		maxAgeDays,
	)

	fmt.Println(
		"[TURKMENPORTAL] Минимальная дата:",
		cutoff.Format("2006-01-02"),
	)

	for page := 1; page <= maxPages; page++ {

		fmt.Printf(
			"[TURKMENPORTAL] ===== Страница %d =====\n",
			page,
		)

		c := colly.NewCollector(
			colly.AllowURLRevisit(),
		)
		c.SetRequestTimeout(30 * time.Second)

		var callbackErr error
		oldArticleFound := false
		newArticles := 0

		c.OnRequest(func(r *colly.Request) {
			fmt.Println(
				"[TURKMENPORTAL] Открываю список:",
				r.URL,
			)
		})

		c.OnHTML(
			`a[href^="/ru/news/"]`,
			func(e *colly.HTMLElement) {

				if callbackErr != nil {
					return
				}

				href := e.Attr("href")

				parts := strings.Split(
					strings.Trim(href, "/"),
					"/",
				)

				if len(parts) != 3 {
					return
				}

				slug := parts[2]

				slugParts := strings.SplitN(
					slug,
					"-",
					2,
				)

				if len(slugParts) != 2 {
					return
				}

				externalID := slugParts[0]

				_, err := strconv.ParseInt(
					externalID,
					10,
					64,
				)
				if err != nil {
					return
				}

				if seen[externalID] {
					return
				}

				seen[externalID] = true

				articleURL := e.Request.AbsoluteURL(href)

				fmt.Println(
					"[TURKMENPORTAL] Найдена статья:",
					articleURL,
				)

				article := model.Article{
					ExternalID: externalID,
					SourceName: "turkmenportal",
					ScrapedAt:  time.Now(),
					Published:  true,
				}

				alternateURLs := make(map[string]string)

				err = scrapeArticlePage(
					articleURL,
					"ru",
					&article,
					alternateURLs,
				)
				if err != nil {
					fmt.Println(
						"[TURKMENPORTAL] Ошибка RU:",
						err,
					)
					return
				}

				if article.PostedAt != nil &&
					article.PostedAt.Before(cutoff) {

					fmt.Println(
						"[TURKMENPORTAL] Статья старше допустимого срока:",
						externalID,
					)

					oldArticleFound = true
					return
				}

				newArticles++

				tmURL, ok := alternateURLs["tm"]
				if ok {
					err = scrapeArticlePage(
						tmURL,
						"tm",
						&article,
						nil,
					)

					if err != nil {
						fmt.Println(
							"[TURKMENPORTAL] TM перевод недоступен:",
							err,
						)
					}
				}

				enURL, ok := alternateURLs["en"]
				if ok {
					err = scrapeArticlePage(
						enURL,
						"en",
						&article,
						nil,
					)

					if err != nil {
						fmt.Println(
							"[TURKMENPORTAL] EN перевод недоступен:",
							err,
						)
					}
				}

				err = repo.SaveArticle(ctx, article)
				if err != nil {
					callbackErr = fmt.Errorf(
						"сохранение статьи %s: %w",
						externalID,
						err,
					)
					return
				}

				fmt.Println(
					"[TURKMENPORTAL] Статья сохранена:",
					externalID,
				)

			},
		)

		var listURL string

		if page == 1 {
			listURL = "https://turkmenportal.com/ru/news"
		} else {
			listURL = fmt.Sprintf(
				"https://turkmenportal.com/ru/news?page=%d",
				page,
			)
		}

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

		if oldArticleFound {
			fmt.Println(
				"[TURKMENPORTAL] Найдены старые статьи. Дальше не идём.",
			)
			break
		}

		if newArticles == 0 {
			fmt.Println(
				"[TURKMENPORTAL] На странице нет новых статей. Останавливаюсь.",
			)
			break
		}
	}

	return nil
}

func stringPointer(value string) *string {
	return &value
}

func scrapeArticlePage(articleURL string, language string, article *model.Article, alternateURLs map[string]string) error {
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
	)

	c.SetRequestTimeout(30 * time.Second)

	var title string
	var text string
	var postedAt *time.Time
	var imgURL *string

	if alternateURLs != nil {
		c.OnHTML(`link[rel="alternate"][hreflang]`, func(e *colly.HTMLElement) {
			hreflang := strings.ToLower(strings.TrimSpace(e.Attr("hreflang")))
			href := strings.TrimSpace(e.Attr("href"))

			if href == "" {
				return
			}

			href = e.Request.AbsoluteURL(href)

			switch hreflang {
			case "ru-ru":
				alternateURLs["ru"] = href
			case "tk-tm":
				alternateURLs["tm"] = href
			case "en-us":
				alternateURLs["en"] = href
			}
		})
	}

	if article.Category == nil {
		c.OnHTML(`a[href*="/news/category/"]`, func(e *colly.HTMLElement) {
			if article.Category != nil {
				return
			}

			href := strings.TrimSpace(e.Attr("href"))

			const marker = "/news/category/"

			index := strings.Index(href, marker)
			if index == -1 {
				return
			}

			categoryPart := href[index+len(marker):]
			categoryPart = strings.Trim(categoryPart, "/")

			parts := strings.SplitN(categoryPart, "-", 2)
			if len(parts) != 2 {
				return
			}

			category := normalizeCategory(parts[1])

			article.Category = stringPointer(category)

			fmt.Println(
				"[TURKMENPORTAL] Категория:",
				category,
			)
		})
	}

	c.OnHTML(`meta[property="og:title"]`, func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Attr("content"))
	})

	/* c.OnHTML("main div.text-3xl.font-bold", func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Text)
	}) */

	c.OnHTML(`meta[property="og:image"]`, func(e *colly.HTMLElement) {
		value := strings.TrimSpace(e.Attr("content"))
		if value != "" {
			imgURL = &value
		}
	})

	c.OnHTML(`meta[property="article:published_time"]`, func(e *colly.HTMLElement) {
		rawDate := strings.TrimSpace(e.Attr("content"))

		value, err := time.Parse(time.RFC3339Nano, rawDate)
		if err != nil {
			return
		}

		postedAt = &value
	})

	c.OnHTML("#content", func(e *colly.HTMLElement) {
		paragraphs := e.ChildTexts("p")

		var cleanParagraphs []string

		for _, paragraph := range paragraphs {
			paragraph = strings.TrimSpace(paragraph)

			if paragraph == "" {
				continue
			}

			cleanParagraphs = append(cleanParagraphs, paragraph)
		}

		text = strings.Join(cleanParagraphs, "\n\n")
	})

	err := visitWithRetry(
		c,
		articleURL,
		3,
	)
	if err != nil {
		return err
	}

	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("не найден заголовок на языке %s: %s", language, articleURL)
	}

	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("не найден текст на языке %s: %s", language, articleURL)
	}

	switch language {
	case "ru":
		article.TitleRU = stringPointer(title)
		article.TextRU = stringPointer(text)
		article.URLRU = stringPointer(articleURL)

	case "tm":
		article.TitleTM = stringPointer(title)
		article.TextTM = stringPointer(text)
		article.URLTM = stringPointer(articleURL)

	case "en":
		article.TitleEN = stringPointer(title)
		article.TextEN = stringPointer(text)
		article.URLEN = stringPointer(articleURL)
	}

	if article.PostedAt == nil && postedAt != nil {
		article.PostedAt = postedAt
	}

	if article.ImgURL == nil && imgURL != nil {
		article.ImgURL = imgURL
	}

	return nil
}

func normalizeCategory(category string) string {
	categories := map[string]string{
		"kultura":     "culture",
		"ekonomika":   "economy",
		"obrazovanie": "education",
		"v-mire":      "world",
		"sport":       "sport",
		"turizm":      "tourism",
		"tekhnologii": "technology",
		"zdorove":     "health",
		"energetika":  "energy",
		"obshchestvo": "society",
		"transport":   "transport",
		"biznes":      "business",
		"politika":    "politics",
	}

	value, ok := categories[category]
	if ok {
		return value
	}

	return category
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
				"[TURKMENPORTAL] Ошибка запроса. Повтор %d/%d: %v\n",
				attempt+1,
				maxAttempts,
				err,
			)

			time.Sleep(2 * time.Second)
		}
	}

	return lastErr
}
