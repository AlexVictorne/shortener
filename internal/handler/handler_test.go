package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shortener/internal/handler"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
)

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
			name:        "method not allowed",
			method:      http.MethodGet,
			contentType: "text/plain",
			body:        "https://ya.ru",
			wantStatus:  http.StatusMethodNotAllowed,
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
			wantStatus:  http.StatusInternalServerError,
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
			if res.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantInResponse != "" {
				b, _ := io.ReadAll(res.Body)
				if !strings.Contains(string(b), tt.wantInResponse) {
					t.Errorf("response body = %q, want substring %q", string(b), tt.wantInResponse)
				}
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
			name:       "method not allowed",
			method:     http.MethodPost,
			path:       "/" + id,
			wantStatus: http.StatusMethodNotAllowed,
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
			if res.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantLoc != "" {
				loc := res.Header.Get("Location")
				if loc != tt.wantLoc {
					t.Errorf("Location header = %q, want %q", loc, tt.wantLoc)
				}
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
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}
