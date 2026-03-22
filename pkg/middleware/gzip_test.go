package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGzipMiddleware_CompressesResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello, world"))
	})

	wrapped := GzipMiddleware(handler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("expected gzip encoding, got %q", resp.Header.Get("Content-Encoding"))
	}
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()
	uncompressed, err := io.ReadAll(gzr)
	if err != nil {
		t.Fatalf("failed to read gzip body: %v", err)
	}
	if string(uncompressed) != "hello, world" {
		t.Errorf("unexpected response body: %q", string(uncompressed))
	}
}

func TestGzipMiddleware_DecompressesRequest(t *testing.T) {
	var got string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	})

	wrapped := GzipMiddleware(handler)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("test body"))
	gz.Close()

	req := httptest.NewRequest("POST", "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if got != "test body" {
		t.Errorf("expected body to be decompressed, got %q", got)
	}
}

func TestGzipMiddleware_PassThrough(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain response"))
	})

	wrapped := GzipMiddleware(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.Header.Get("Content-Encoding") == "gzip" {
		t.Errorf("did not expect gzip encoding")
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain response" {
		t.Errorf("unexpected response body: %q", string(body))
	}
}
