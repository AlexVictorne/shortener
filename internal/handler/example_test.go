package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"shortener/internal/handler"
	"shortener/internal/handler/options"
	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
	"shortener/pkg/middleware"
)

const exampleSecret = "example-secret"

// reqCtxWithUser возвращает контекст с произвольным userID для прямых вызовов сервиса в примерах.
func reqCtxWithUser() context.Context { //nolint:revive
	return context.WithValue(context.Background(), middleware.UserIDKey, "example-user")
}

// newExampleHandler создаёт Handler со свежим in-memory хранилищем для примеров.
func newExampleHandler() (*handler.Handler, *service.TrimmerService) {
	storage := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(storage, gen, "http://localhost:8080/")
	h := handler.NewHandler(svc, options.WithAuthSecret(exampleSecret))
	return h, svc
}

// withAuth оборачивает HandlerFunc в AuthMiddleware, чтобы запрос имел userID в контексте.
func withAuth(hf http.HandlerFunc) http.Handler {
	return middleware.AuthMiddleware(exampleSecret)(hf)
}

// Example_shortenURLHandler демонстрирует POST / — сокращение URL через text/plain.
func Example_shortenURLHandler() {
	h, _ := newExampleHandler()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com/long/path"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	withAuth(h.ShortenURLHandler).ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	fmt.Println("Status:", res.StatusCode)
	fmt.Println("Has short URL:", strings.HasPrefix(string(body), "http://localhost:8080/"))
	// Output:
	// Status: 201
	// Has short URL: true
}

// Example_shortenURLJSONHandler демонстрирует POST /api/shorten — сокращение URL через JSON.
func Example_shortenURLJSONHandler() {
	h, _ := newExampleHandler()

	body := `{"url":"https://example.com/another/long/path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	withAuth(h.ShortenURLJSONHandler).ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	var resp struct {
		Result string `json:"result"`
	}
	_ = json.NewDecoder(res.Body).Decode(&resp)

	fmt.Println("Status:", res.StatusCode)
	fmt.Println("Has short URL:", strings.HasPrefix(resp.Result, "http://localhost:8080/"))
	// Output:
	// Status: 201
	// Has short URL: true
}

// Example_batchShortenHandler демонстрирует POST /api/shorten/batch — пакетное сокращение.
func Example_batchShortenHandler() {
	h, _ := newExampleHandler()

	items := []model.BatchRequestItem{
		{CorrelationID: "c1", OriginalURL: "https://go.dev/doc"},
		{CorrelationID: "c2", OriginalURL: "https://pkg.go.dev"},
	}
	data, _ := json.Marshal(items)

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	withAuth(h.BatchShortenHandler).ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	var resp []model.BatchResponseItem
	_ = json.NewDecoder(res.Body).Decode(&resp)

	fmt.Println("Status:", res.StatusCode)
	fmt.Println("Count:", len(resp))
	fmt.Println("CorrelationIDs match:", resp[0].CorrelationID == "c1" && resp[1].CorrelationID == "c2")
	// Output:
	// Status: 201
	// Count: 2
	// CorrelationIDs match: true
}

// Example_redirectHandler демонстрирует GET /{id} — переход по короткому URL.
func Example_redirectHandler() {
	h, svc := newExampleHandler()

	// Сначала сокращаем URL, чтобы получить короткий идентификатор.
	shortFull, _ := svc.TrimURL(reqCtxWithUser(), "https://example.com/redirect-target")
	id := shortFull[len("http://localhost:8080/"):]

	mux := chi.NewRouter()
	mux.Use(middleware.AuthMiddleware(exampleSecret))
	mux.Get("/{id}", h.RedirectHandler)

	req := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	fmt.Println("Status:", res.StatusCode)
	fmt.Println("Location:", res.Header.Get("Location"))
	// Output:
	// Status: 307
	// Location: https://example.com/redirect-target
}

// Example_getUserURLsHandler демонстрирует GET /api/user/urls — список URL пользователя.
func Example_getUserURLsHandler() {
	h, _ := newExampleHandler()

	// Создаём короткую ссылку от имени пользователя через HTTP (чтобы userID попал в хранилище).
	createReq := httptest.NewRequest(http.MethodPost, "/api/shorten",
		strings.NewReader(`{"url":"https://example.com/my-page"}`))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	withAuth(h.ShortenURLJSONHandler).ServeHTTP(cw, createReq)

	// Извлекаем auth-куку, выданную при создании.
	createRes := cw.Result()
	defer createRes.Body.Close()
	authCookie := createRes.Cookies()[0]

	// Запрашиваем список URL с той же кукой.
	listReq := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	listReq.AddCookie(authCookie)
	lw := httptest.NewRecorder()
	withAuth(h.GetUserURLsHandler).ServeHTTP(lw, listReq)

	res := lw.Result()
	defer res.Body.Close()

	var urls []model.UserURLResponse
	_ = json.NewDecoder(res.Body).Decode(&urls)

	fmt.Println("Status:", res.StatusCode)
	fmt.Println("Count:", len(urls))
	fmt.Println("OriginalURL:", urls[0].OriginalURL)
	// Output:
	// Status: 200
	// Count: 1
	// OriginalURL: https://example.com/my-page
}

// Example_deleteUserURLsHandler демонстрирует DELETE /api/user/urls — мягкое удаление ссылок.
func Example_deleteUserURLsHandler() {
	h, _ := newExampleHandler()

	// Создаём ссылку.
	createReq := httptest.NewRequest(http.MethodPost, "/api/shorten",
		strings.NewReader(`{"url":"https://example.com/to-delete"}`))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	withAuth(h.ShortenURLJSONHandler).ServeHTTP(cw, createReq)

	createRes := cw.Result()
	defer createRes.Body.Close()
	var created struct{ Result string }
	_ = json.NewDecoder(createRes.Body).Decode(&created)
	id := created.Result[len("http://localhost:8080/"):]
	authCookie := createRes.Cookies()[0]

	// Удаляем ссылку.
	ids, _ := json.Marshal([]string{id})
	delReq := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(string(ids)))
	delReq.Header.Set("Content-Type", "application/json")
	delReq.AddCookie(authCookie)
	dw := httptest.NewRecorder()
	withAuth(h.DeleteUserURLsHandler).ServeHTTP(dw, delReq)

	fmt.Println("Status:", dw.Code)
	// Output:
	// Status: 202
}

// Example_pingHandler демонстрирует GET /ping — проверку доступности хранилища.
// Без PostgreSQL pinger не задан, поэтому ожидается 500.
func Example_pingHandler() {
	h, _ := newExampleHandler() // pinger не задан — используется MemStorage

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	h.PingHandler(w, req)

	fmt.Println("Status:", w.Code)
	// Output:
	// Status: 500
}
