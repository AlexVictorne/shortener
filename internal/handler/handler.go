// Package handler реализует HTTP-слой сервиса сокращения ссылок.
// Связывает TrimmerService, middleware аутентификации, аудит-логирование
// и Chi-роутер, предоставляя следующие эндпоинты:
//
//	POST /                   — сократить URL (тело text/plain)
//	POST /api/shorten        — сократить URL (тело JSON)
//	POST /api/shorten/batch  — пакетное сокращение URL
//	GET  /{id}               — перенаправление на оригинальный URL
//	GET  /api/user/urls      — список URL текущего пользователя
//	DELETE /api/user/urls    — мягкое удаление коротких ссылок
//	GET  /ping               — проверка доступности хранилища
//	GET  /api/internal/stats — статистика сервиса (только из доверенной подсети)
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"shortener/internal/model"
	"shortener/internal/service"
	"shortener/pkg/audit"
	"shortener/pkg/middleware"

	"shortener/internal/handler/options"
)

// Pinger реализуется любым бэкендом хранилища, способным проверить собственную доступность.
// PgStorage удовлетворяет этому интерфейсу; MemStorage не требует отдельного пинга.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler хранит зависимости, необходимые для обработки HTTP-запросов.
// Создавайте экземпляры через NewHandler; не конструируйте структуру напрямую.
type Handler struct {
	service    *service.TrimmerService
	pinger     Pinger
	authSecret string
	auditor    audit.Auditor
	// trustedNet — разобранная CIDR доверенной подсети; nil означает запрет доступа к /api/internal/stats.
	trustedNet *net.IPNet
}

// NewHandler создаёт Handler с переданным TrimmerService и функциональными опциями.
// Доступные опции: options.WithAuthSecret, options.WithPinger, options.WithAuditor.
func NewHandler(service *service.TrimmerService, opts ...options.OptHandlerOptionsSetter) *Handler {
	optsStruct := options.NewHandlerOptions(opts...)
	h := &Handler{
		service: service,
		auditor: audit.NoopAuditor{},
	}
	if optsStruct.AuthSecret != "" {
		h.authSecret = optsStruct.AuthSecret
	}
	if optsStruct.Pinger != nil {
		if p, ok := optsStruct.Pinger.(Pinger); ok {
			h.pinger = p
		}
	}
	if optsStruct.Auditor != nil {
		if a, ok := optsStruct.Auditor.(audit.Auditor); ok {
			h.auditor = a
		}
	}
	if optsStruct.TrustedSubnet != "" {
		_, ipNet, err := net.ParseCIDR(optsStruct.TrustedSubnet)
		if err != nil {
			log.Warn().Str("cidr", optsStruct.TrustedSubnet).Msg("invalid trusted subnet CIDR, stats endpoint will be disabled")
		} else {
			h.trustedNet = ipNet
		}
	}
	return h
}

func (h *Handler) emitAudit(ctx context.Context, action, userID, url string) {
	if err := h.auditor.Emit(ctx, audit.Event{
		TS:     time.Now().Unix(),
		Action: action,
		UserID: userID,
		URL:    url,
	}); err != nil {
		log.Warn().Err(err).Msg("audit emit failed")
	}
}

// ShortenURLHandler обрабатывает POST / с телом text/plain, содержащим оригинальный URL.
// При успехе отвечает 201 Created с коротким URL в виде обычного текста.
// Если URL уже был сокращён, отвечает 409 Conflict, но всё равно возвращает
// существующий короткий URL, чтобы вызывающая сторона могла его использовать.
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
		switch {
		case errors.Is(err, service.ErrConflict):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(shortURL))
			return
		case errors.Is(err, service.ErrURLEmpty),
			errors.Is(err, service.ErrURLTooLong),
			errors.Is(err, service.ErrURLInvalidFormat),
			errors.Is(err, service.ErrURLInvalidScheme),
			errors.Is(err, service.ErrURLNoHost):
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		default:
			log.Printf("TrimURL error: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	userID, _ := middleware.UserIDFromContext(r.Context())
	h.emitAudit(r.Context(), "shorten", userID, string(body))
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

// ShortenURLJSONHandler обрабатывает POST /api/shorten с JSON-телом {"url": "…"}.
// При успехе отвечает 201 Created с телом {"result": "<short_url>"}.
// При дублировании URL возвращает 409 Conflict с тем же JSON-телом,
// чтобы вызывающая сторона всё равно получила короткий URL.
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
		switch {
		case errors.Is(err, service.ErrConflict):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			resp := shortenResponse{Result: shortURL}
			json.NewEncoder(w).Encode(resp)
			return
		case errors.Is(err, service.ErrURLEmpty),
			errors.Is(err, service.ErrURLTooLong),
			errors.Is(err, service.ErrURLInvalidFormat),
			errors.Is(err, service.ErrURLInvalidScheme),
			errors.Is(err, service.ErrURLNoHost):
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		default:
			log.Printf("TrimURL error: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	userID, _ := middleware.UserIDFromContext(r.Context())
	h.emitAudit(r.Context(), "shorten", userID, req.URL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	resp := shortenResponse{Result: shortURL}
	json.NewEncoder(w).Encode(resp)
}

// GetUserURLsHandler обрабатывает GET /api/user/urls.
// Требует валидную auth-куку; при её отсутствии возвращает 401 Unauthorized.
// Отвечает 200 OK с JSON-массивом [{"short_url":…,"original_url":…}]
// или 204 No Content, если у пользователя нет сохранённых URL.
func (h *Handler) GetUserURLsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	urls, err := h.service.GetURLsByUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrNoContent) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(urls)
}

// RedirectHandler обрабатывает GET /{id}.
// Разрешает короткий идентификатор в оригинальный URL и выполняет 307 Temporary Redirect.
// Возвращает 410 Gone, если URL был мягко удалён, или 404 Not Found, если id неизвестен.
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
		if errors.Is(err, service.ErrURLDeleted) {
			http.Error(w, http.StatusText(http.StatusGone), http.StatusGone)
			return
		}
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	userID, _ := middleware.UserIDFromContext(r.Context())
	h.emitAudit(r.Context(), "follow", userID, originalURL)
	http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
}

// NotFoundHandler — резервный обработчик 404, зарегистрированный в Chi-роутере.
func (h *Handler) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

// MethodNotAllowedHandler — обработчик 405, зарегистрированный в Chi-роутере.
func (h *Handler) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}

// PingHandler обрабатывает GET /ping и проверяет доступность хранилища.
// Возвращает 200 OK при успехе или 500 Internal Server Error при ошибке соединения
// либо если Pinger не задан (используется MemStorage без PostgreSQL).
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

// DeleteUserURLsHandler обрабатывает DELETE /api/user/urls.
// Принимает JSON-массив коротких идентификаторов и запускает асинхронное мягкое удаление.
// Немедленно отвечает 202 Accepted; фактическое удаление происходит в фоне.
func (h *Handler) DeleteUserURLsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	var ids []string
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if err := dec.Decode(&ids); err != nil || len(ids) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	_ = h.service.MarkURLsDeleted(r.Context(), userID, ids)
	w.WriteHeader(http.StatusAccepted)
}

// statsResponse — тело ответа эндпоинта GET /api/internal/stats.
type statsResponse struct {
	// URLs — общее количество сокращенных URL в сервисе.
	URLs int `json:"urls"`
	// Users — общее количество пользователей в сервисе.
	Users int `json:"users"`
}

// StatsHandler обрабатывает GET /api/internal/stats.
// Возвращает JSON-объект с количеством URL и пользователей.
// Доступ разрешен только клиентам, чей IP (из заголовка X-Real-IP) входит в доверенную подсеть.
// При пустом trusted_subnet или несоответствии IP возвращает 403 Forbidden.
func (h *Handler) StatsHandler(w http.ResponseWriter, r *http.Request) {
	// Если доверенная подсеть не задана — запрещаем любой доступ
	if h.trustedNet == nil {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	rawIP := r.Header.Get("X-Real-IP")
	ip := net.ParseIP(rawIP)
	if ip == nil || !h.trustedNet.Contains(ip) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	urls, users, err := h.service.Stats(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("stats: storage error")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(statsResponse{URLs: urls, Users: users}); err != nil {
		log.Error().Err(err).Msg("stats: encode error")
	}
}

// SetupRoutes регистрирует все маршруты и middleware в переданном Chi-роутере.
// Порядок middleware: RequestResponseLogger → GzipMiddleware → AuthMiddleware.
func (h *Handler) SetupRoutes(mux chi.Router) {
	mux.Use(middleware.RequestResponseLogger)
	mux.Use(middleware.GzipMiddleware)
	mux.Use(middleware.AuthMiddleware(h.authSecret))

	mux.Get("/ping", h.PingHandler)
	mux.Get("/api/internal/stats", h.StatsHandler)
	mux.Post("/", h.ShortenURLHandler)
	mux.Post("/api/shorten", h.ShortenURLJSONHandler)
	mux.Post("/api/shorten/batch", h.BatchShortenHandler)
	mux.Get("/api/user/urls", h.GetUserURLsHandler)
	mux.Delete("/api/user/urls", h.DeleteUserURLsHandler)
	mux.Get("/{id}", h.RedirectHandler)
	mux.NotFound(h.NotFoundHandler)
	mux.MethodNotAllowed(h.MethodNotAllowedHandler)
}

// BatchShortenHandler обрабатывает POST /api/shorten/batch.
// Принимает JSON-массив объектов BatchRequestItem и возвращает JSON-массив BatchResponseItem.
// При успехе отвечает 201 Created. Если хотя бы один элемент невалиден, возвращает 400 Bad Request.
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

	resp, err := h.service.BatchShorten(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBatchItemEmpty):
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		case errors.Is(err, service.ErrURLEmpty),
			errors.Is(err, service.ErrURLTooLong),
			errors.Is(err, service.ErrURLInvalidFormat),
			errors.Is(err, service.ErrURLInvalidScheme),
			errors.Is(err, service.ErrURLNoHost):
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		default:
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
