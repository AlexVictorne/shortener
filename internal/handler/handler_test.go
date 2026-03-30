package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shortener/internal/handler"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
)

func TestHandler_ShortenURLJSONHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc)

	tests := []struct {
		name            string
		method          string
		contentType     string
		body            string
		wantStatus      int
		wantInResponse  string
		wantContentType string
	}{
		{
			name:            "success",
			method:          http.MethodPost,
			contentType:     "application/json",
			body:            `{"url": "https://ya.ru"}`,
			wantStatus:      http.StatusCreated,
			wantInResponse:  "http://localhost:8080/",
			wantContentType: "application/json",
		},
		{
			name:        "bad content type",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        `{"url": "https://ya.ru"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty url",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"url": ""}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "invalid json",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"url":`,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/shorten", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			h.ShortenURLJSONHandler(w, req)
			res := w.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.wantStatus, res.StatusCode)

			if tt.wantInResponse != "" {
				b, _ := io.ReadAll(res.Body)
				assert.Contains(t, string(b), tt.wantInResponse)
			}
			if tt.wantContentType != "" {
				assert.Equal(t, tt.wantContentType, res.Header.Get("Content-Type"))
			}
		})
	}
}

func TestHandler_ShortenURLHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		wantStatus     int
		wantInResponse string
	}{
		{
			name:           "success",
			method:         http.MethodPost,
			contentType:    "text/plain",
			body:           "https://ya.ru",
			wantStatus:     http.StatusCreated,
			wantInResponse: "http://localhost:8080/",
		},
		{
			name:        "bad content type",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        "https://ya.ru",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "bad body",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        string(make([]byte, 3000)), // too large and internal error from service
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			h.ShortenURLHandler(w, req)
			res := w.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.wantStatus, res.StatusCode)

			if tt.wantInResponse != "" {
				b, _ := io.ReadAll(res.Body)

				assert.Contains(t, string(b), tt.wantInResponse)
			}
		})
	}
}

func TestHandler_RedirectHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc)

	// Сначала сохраним URL
	shortURL, _ := svc.TrimURL(context.TODO(), "https://ya.ru")
	id := strings.TrimPrefix(shortURL, "http://localhost:8080/")

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantLoc    string
	}{
		{
			name:       "success",
			method:     http.MethodGet,
			path:       "/" + id,
			wantStatus: http.StatusTemporaryRedirect,
			wantLoc:    "https://ya.ru",
		},
		{
			name:       "bad id",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			method:     http.MethodGet,
			path:       "/notexist",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			h.RedirectHandler(w, req)
			res := w.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.wantStatus, res.StatusCode)

			if tt.wantLoc != "" {
				loc := res.Header.Get("Location")
				assert.Equal(t, tt.wantLoc, loc)
			}
		})
	}
}

func TestHandler_NotFoundHandler(t *testing.T) {
	svc := service.NewTrimmerService(repository.NewMemStorage(), generator.NewGenerator(8), "http://localhost:8080/")
	h := handler.NewHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	h.NotFoundHandler(w, req)
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d, want %d", res.StatusCode, http.StatusNotFound)
	}
}

func testRequest(t *testing.T, ts *httptest.Server, method, path string, contentType string) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)

	req.Header.Set("Content-Type", contentType)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

func TestHandler_Router(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc)

	r := chi.NewRouter()
	h.SetupRoutes(r)

	ts := httptest.NewServer(r)
	defer ts.Close()

	var tests = []struct {
		name        string
		url         string
		contentType string
		method      string
		want        string
		status      int
	}{
		{name: "empty GET", url: "/", method: http.MethodGet, want: http.StatusText(http.StatusMethodNotAllowed), status: http.StatusMethodNotAllowed},
		{name: "not found GET", url: "/abc12345", method: http.MethodGet, want: http.StatusText(http.StatusNotFound), status: http.StatusNotFound},
		{name: "other adress GET", url: "/yaopo/oi", method: http.MethodGet, want: http.StatusText(http.StatusNotFound), status: http.StatusNotFound},
		{name: "unsupported type POST", url: "/", contentType: "application/json", method: http.MethodPost, want: http.StatusText(http.StatusBadRequest), status: http.StatusBadRequest},
		{name: "empty POST", url: "/", contentType: "text/plain", method: http.MethodPost, want: http.StatusText(http.StatusInternalServerError), status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := testRequest(t, ts, tt.method, tt.url, tt.contentType)
			defer resp.Body.Close()

			assert.Equal(t, tt.status, resp.StatusCode)

			if tt.want != "" {
				assert.Contains(t, body, tt.want)
			}
		})
	}
}

type mockPinger struct{ err error }

func (m *mockPinger) Ping(ctx context.Context) error { return m.err }

func TestHandler_PingHandler(t *testing.T) {
	svc := service.NewTrimmerService(repository.NewMemStorage(), generator.NewGenerator(8), "http://localhost:8080/")

	t.Run("no pinger", func(t *testing.T) {
		h := handler.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.PingHandler(w, req)
		res := w.Result()
		defer res.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	})

	t.Run("pinger ok", func(t *testing.T) {
		h := handler.NewHandler(svc).WithPinger(&mockPinger{err: nil})
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.PingHandler(w, req)
		res := w.Result()
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("pinger error", func(t *testing.T) {
		h := handler.NewHandler(svc).WithPinger(&mockPinger{err: assert.AnError})
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.PingHandler(w, req)
		res := w.Result()
		defer res.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	})
}

func TestHandler_BatchShortenHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		wantStatus     int
		wantInResponse string
	}{
		{
			name:           "success batch",
			method:         http.MethodPost,
			contentType:    "application/json",
			body:           `[{"correlation_id":"1","original_url":"https://ya.ru"},{"correlation_id":"2","original_url":"https://yandex.ru"}]`,
			wantStatus:     http.StatusCreated,
			wantInResponse: "short_url",
		},
		{
			name:        "empty batch",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[]`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "bad content type",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        `[{"correlation_id":"1","original_url":"https://ya.ru"}]`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing correlation_id",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[{"original_url":"https://ya.ru"}]`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing original_url",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[{"correlation_id":"1"}]`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "invalid json",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "duplicate url in batch",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[{"correlation_id":"1","original_url":"https://ya.ru"},{"correlation_id":"2","original_url":"https://ya.ru"}]`,
			wantStatus:  http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/shorten/batch", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			h.BatchShortenHandler(w, req)
			res := w.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.wantStatus, res.StatusCode)

			if tt.wantInResponse != "" && res.StatusCode == http.StatusCreated {
				b, _ := io.ReadAll(res.Body)
				assert.Contains(t, string(b), tt.wantInResponse)
			}
		})
	}
}
