package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shortener/internal/handler"
	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
	"shortener/pkg/middleware"
)

func TestHandler_ShortenURLJSONHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := newTestHandler(svc)

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
			wrapWithAuth(h.ShortenURLJSONHandler).ServeHTTP(w, req)
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
	h := newTestHandler(svc)

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
			wrapWithAuth(h.ShortenURLHandler).ServeHTTP(w, req)
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
	ctx := context.WithValue(context.TODO(), middleware.UserIDKey, "test-user")
	shortURL, _ := svc.TrimURL(ctx, "https://ya.ru")
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
		{name: "empty POST", url: "/", contentType: "text/plain", method: http.MethodPost, want: http.StatusText(http.StatusBadRequest), status: http.StatusBadRequest},
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
	h := newTestHandler(svc)

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
			wrapWithAuth(h.BatchShortenHandler).ServeHTTP(w, req)
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

const testSecret = "test-secret"

func newTestHandler(svc *service.TrimmerService) *handler.Handler {
	return handler.NewHandler(svc).WithAuthSecret(testSecret)
}

func wrapWithAuth(h http.HandlerFunc) http.Handler {
	return middleware.AuthMiddleware(testSecret)(h)
}

func TestHandler_GetUserURLsHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := newTestHandler(svc)

	req := httptest.NewRequest("GET", "/api/user/urls", nil)
	rw := httptest.NewRecorder()
	h.GetUserURLsHandler(rw, req)
	resp := rw.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req2 := httptest.NewRequest("GET", "/api/user/urls", nil)
	rw2 := httptest.NewRecorder()
	wrapWithAuth(h.GetUserURLsHandler).ServeHTTP(rw2, req2)
	resp2 := rw2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp2.StatusCode)

	req3 := httptest.NewRequest("POST", "/api/shorten", strings.NewReader("https://ya.ru"))
	req3.Header.Set("Content-Type", "text/plain")
	rw3 := httptest.NewRecorder()
	wrapWithAuth(h.ShortenURLHandler).ServeHTTP(rw3, req3)
	resp3 := rw3.Result()
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusCreated, resp3.StatusCode)

	// Получаем auth_token из Set-Cookie
	var authToken string
	for _, c := range resp3.Cookies() {
		if c.Name == "auth_token" {
			authToken = c.Value
			break
		}
	}
	if authToken == "" {
		t.Fatal("auth_token cookie not set after POST /api/shorten")
	}

	req4 := httptest.NewRequest("GET", "/api/user/urls", nil)
	req4.AddCookie(&http.Cookie{Name: "auth_token", Value: authToken})
	rw4 := httptest.NewRecorder()
	wrapWithAuth(h.GetUserURLsHandler).ServeHTTP(rw4, req4)
	resp4 := rw4.Result()
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusOK, resp4.StatusCode)
	assert.Equal(t, "application/json", resp4.Header.Get("Content-Type"))
	b, _ := io.ReadAll(resp4.Body)
	var urls []model.UserURLResponse
	_ = json.Unmarshal(b, &urls)
	assert.GreaterOrEqual(t, len(urls), 1)
	assert.Contains(t, urls[0].OriginalURL, "https://ya.ru")
}

func TestHandler_DeleteUserURLsHandler(t *testing.T) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := newTestHandler(svc)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url": "https://yandex.ru"}`))
	reqCreate.Header.Set("Content-Type", "application/json")
	rwCreate := httptest.NewRecorder()
	wrapWithAuth(h.ShortenURLJSONHandler).ServeHTTP(rwCreate, reqCreate)
	respCreate := rwCreate.Result()
	defer respCreate.Body.Close()
	assert.Equal(t, http.StatusCreated, respCreate.StatusCode)

	var authToken string
	for _, c := range respCreate.Cookies() {
		if c.Name == "auth_token" {
			authToken = c.Value
			break
		}
	}
	if authToken == "" {
		t.Fatal("auth_token cookie not set after POST /api/shorten")
	}

	var respData struct {
		Result string `json:"result"`
	}
	b, _ := io.ReadAll(respCreate.Body)
	err := json.Unmarshal(b, &respData)
	require.NoError(t, err)
	shortURL := respData.Result
	parts := strings.Split(shortURL, "/")
	id := parts[len(parts)-1]

	body := `["` + id + `"]`
	req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: authToken})
	rw := httptest.NewRecorder()
	wrapWithAuth(h.DeleteUserURLsHandler).ServeHTTP(rw, req)
	resp := rw.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var deleted bool
	for i := 0; i < 1000; i++ {
		u, _ := storage.Get(context.Background(), id)
		if u != nil && u.DeletedFlag {
			deleted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !deleted {
		t.Fatal("DeletedFlag was not set after waiting")
	}

	reqGone := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	rwGone := httptest.NewRecorder()
	h.RedirectHandler(rwGone, reqGone)
	respGone := rwGone.Result()
	defer respGone.Body.Close()
	assert.Equal(t, http.StatusGone, respGone.StatusCode)

	req2 := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(body))
	rw2 := httptest.NewRecorder()
	h.DeleteUserURLsHandler(rw2, req2)
	resp2 := rw2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)

	req3 := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader("{"))
	rw3 := httptest.NewRecorder()
	wrapWithAuth(h.DeleteUserURLsHandler).ServeHTTP(rw3, req3)
	resp3 := rw3.Result()
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp3.StatusCode)

	url2 := &model.ShortURL{ShortURL: "other123", OriginalURL: "https://ya.ru", UserID: "user2"}
	_ = storage.Create(context.Background(), url2)
	bodyOther := `["other123"]`
	req4 := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(bodyOther))
	rw4 := httptest.NewRecorder()
	wrapWithAuth(h.DeleteUserURLsHandler).ServeHTTP(rw4, req4)
	resp4 := rw4.Result()
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp4.StatusCode)
	u2, _ := storage.Get(context.Background(), "other123")
	assert.False(t, u2.DeletedFlag)
}
