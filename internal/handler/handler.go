package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"shortener/internal/service"
	"shortener/pkg/middleware"
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
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	shortURL, err := h.service.TrimURL(r.Context(), string(body))
	if err != nil {
		log.Printf("TrimURL error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shortURL))
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Result string `json:"result"`
}

// ShortenURLJSONHandler handles POST /api/shorten with JSON body {"url": "..."}
func (h *Handler) ShortenURLJSONHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var req shortenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	if err := dec.Decode(&req); err != nil || req.URL == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	shortURL, err := h.service.TrimURL(r.Context(), req.URL)
	if err != nil {
		log.Printf("TrimURL error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	resp := shortenResponse{Result: shortURL}
	json.NewEncoder(w).Encode(resp)
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
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// load original
	originalURL, err := h.service.GetOriginalURL(r.Context(), id)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
}

func (h *Handler) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

func (h *Handler) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}

func (h *Handler) SetupRoutes(mux chi.Router) {
	mux.Use(middleware.RequestResponseLogger)
	mux.Use(middleware.GzipMiddleware)

	mux.Post("/", h.ShortenURLHandler)
	mux.Post("/api/shorten", h.ShortenURLJSONHandler)
	mux.Get("/{id}", h.RedirectHandler)
	mux.NotFound(h.NotFoundHandler)
	mux.MethodNotAllowed(h.MethodNotAllowedHandler)
}
