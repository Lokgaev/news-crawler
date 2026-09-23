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
	mux.HandleFunc("/articles/{id}", s.articleByIDHandler)
	mux.HandleFunc("/categories", s.categoriesHandler)

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

func (s *Server) categoriesHandler(
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

	categories, err := s.repo.ListCategories(
		r.Context(),
	)
	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string][]string{
			"results": categories,
		},
	)
}

func (s *Server) articleByIDHandler(
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

	id, err := strconv.ParseInt(
		r.PathValue("id"),
		10,
		64,
	)

	if err != nil || id <= 0 {
		http.Error(
			w,
			"invalid article id",
			http.StatusBadRequest,
		)
		return
	}

	article, found, err := s.repo.GetArticleByID(
		r.Context(),
		id,
	)

	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	if !found {
		http.Error(
			w,
			"article not found",
			http.StatusNotFound,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		article,
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

	path := r.URL.Path

	if prefix := r.Header.Get("X-Forwarded-Prefix"); prefix != "" {
		path = prefix + path
	}

	return fmt.Sprintf(
		"%s?%s",
		path,
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
