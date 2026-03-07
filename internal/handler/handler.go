package handler

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"

	"shortener/internal/service"
)

type Handler struct {
	service *service.TrimmerService
}

func NewHandler(service *service.TrimmerService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) ShortenURLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "text/plain" {
		http.Error(w, "Unsupported content type", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2048))
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	shortURL, err := h.service.TrimURL(r.Context(), string(body))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shortURL))
}

func (h *Handler) RedirectHandler(w http.ResponseWriter, r *http.Request) {
	// extract id
	var id string
	if chi.RouteContext(r.Context()) != nil {
		id = chi.URLParam(r, "id")
	} else {
		id = r.URL.Path[1:]
	}

	if id == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// load original
	originalURL, err := h.service.GetOriginalURL(r.Context(), id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
}

func (h *Handler) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Bad request", http.StatusBadRequest)
}

func (h *Handler) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) SetupRoutes(mux chi.Router) {
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)

	mux.Post("/", h.ShortenURLHandler)
	mux.Get("/{id}", h.RedirectHandler)
	mux.NotFound(h.NotFoundHandler)
	mux.MethodNotAllowed(h.MethodNotAllowedHandler)
}
