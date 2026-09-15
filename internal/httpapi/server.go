package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"news-crawler/internal/model"
	"news-crawler/internal/repository"
)

type Server struct {
	repo repository.ArticleRepository
}

type ArticlesResponse struct {
	Count    int             `json:"count"`
	Next     *string         `json:"next"`
	Previous *string         `json:"previous"`
	Results  []model.Article `json:"results"`
}

func New(repo repository.ArticleRepository) *Server {
	return &Server{
		repo: repo,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/articles", s.articlesHandler)

	return mux
}

func (s *Server) healthHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status": "ok",
		},
	)
}

func (s *Server) articlesHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	page := 1
	pageSize := 20

	if value := r.URL.Query().Get("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			http.Error(
				w,
				"invalid page",
				http.StatusBadRequest,
			)
			return
		}

		page = parsed
	}

	if value := r.URL.Query().Get("page_size"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(
				w,
				"page_size must be between 1 and 100",
				http.StatusBadRequest,
			)
			return
		}

		pageSize = parsed
	}

	source := r.URL.Query().Get("source")
	category := r.URL.Query().Get("category")
	language := r.URL.Query().Get("lang")

	if language != "" &&
		language != "ru" &&
		language != "en" &&
		language != "tm" {

		http.Error(
			w,
			"lang must be ru, en or tm",
			http.StatusBadRequest,
		)
		return
	}

	offset := (page - 1) * pageSize

	articles, total, err := s.repo.ListArticles(
		r.Context(),
		pageSize,
		offset,
		source,
		category,
		language,
	)

	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	/* for i := range articles {
		articles[i].TextTM = truncateText(articles[i].TextTM, 100)
		articles[i].TextRU = truncateText(articles[i].TextRU, 100)
		articles[i].TextEN = truncateText(articles[i].TextEN, 100)
	} */

	var next *string
	var previous *string

	if offset+len(articles) < total {
		value := buildPageURL(
			r,
			page+1,
			pageSize,
		)

		next = &value
	}

	if page > 1 {
		value := buildPageURL(
			r,
			page-1,
			pageSize,
		)

		previous = &value
	}

	response := ArticlesResponse{
		Count:    total,
		Next:     next,
		Previous: previous,
		Results:  articles,
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

func buildPageURL(
	r *http.Request,
	page int,
	pageSize int,
) string {
	query := r.URL.Query()

	query.Set(
		"page",
		strconv.Itoa(page),
	)

	query.Set(
		"page_size",
		strconv.Itoa(pageSize),
	)

	return fmt.Sprintf(
		"%s?%s",
		r.URL.Path,
		query.Encode(),
	)
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	data any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json; charset=utf-8",
	)

	w.WriteHeader(status)

	encoder := json.NewEncoder(w)

	encoder.SetIndent(
		"",
		"  ",
	)

	_ = encoder.Encode(data)
}

/* func truncateText(text *string, maxLength int) *string {
	if text == nil {
		return nil
	}

	runes := []rune(*text)

	if len(runes) <= maxLength {
		return text
	}

	shortText := string(runes[:maxLength-3]) + "..."

	return &shortText
} */
