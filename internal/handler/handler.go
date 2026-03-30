package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"shortener/internal/model"
	"shortener/internal/service"
	"shortener/pkg/middleware"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	service *service.TrimmerService
	pinger  Pinger
}

func NewHandler(service *service.TrimmerService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) WithPinger(p Pinger) *Handler {
	h.pinger = p
	return h
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
		if errors.Is(err, service.ErrConflict) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(shortURL))
			return
		}
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
		if errors.Is(err, service.ErrConflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			resp := shortenResponse{Result: shortURL}
			json.NewEncoder(w).Encode(resp)
			return
		}
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

func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	if h.pinger == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if err := h.pinger.Ping(r.Context()); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) SetupRoutes(mux chi.Router) {
	mux.Use(middleware.RequestResponseLogger)
	mux.Use(middleware.GzipMiddleware)

	mux.Get("/ping", h.PingHandler)
	mux.Post("/", h.ShortenURLHandler)
	mux.Post("/api/shorten", h.ShortenURLJSONHandler)
	mux.Post("/api/shorten/batch", h.BatchShortenHandler)
	mux.Get("/{id}", h.RedirectHandler)
	mux.NotFound(h.NotFoundHandler)
	mux.MethodNotAllowed(h.MethodNotAllowedHandler)
}

func (h *Handler) BatchShortenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var req []model.BatchRequestItem
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if err := dec.Decode(&req); err != nil || len(req) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	urls := make([]*model.ShortURL, 0, len(req))
	resp := make([]model.BatchResponseItem, 0, len(req))
	for _, item := range req {
		if item.OriginalURL == "" || item.CorrelationID == "" {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if err := h.service.ValidateURL(item.OriginalURL); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		normalized, err := h.service.NormalizeURL(item.OriginalURL)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		existing, err := h.service.GetByOriginal(r.Context(), normalized)
		if err == nil && existing != nil {
			resp = append(resp, model.BatchResponseItem{
				CorrelationID: item.CorrelationID,
				ShortURL:      h.service.BuildShortURL(existing.ShortURL),
			})
			continue
		}
		shortID, err := h.service.GenerateID()
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		urls = append(urls, &model.ShortURL{
			ShortURL:    shortID,
			OriginalURL: normalized,
		})
		resp = append(resp, model.BatchResponseItem{
			CorrelationID: item.CorrelationID,
			ShortURL:      h.service.BuildShortURL(shortID),
		})
	}
	if len(urls) > 0 {
		if err := h.service.BatchCreate(r.Context(), urls); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("failed to encode batch shorten response")
	}
}
