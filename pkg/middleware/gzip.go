package middleware

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      io.Writer
	statusCode  int
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true

	if statusCode < 400 {
		w.Header().Set("Content-Encoding", "gzip")
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	// Не сжимаем ошибочные ответы
	if w.statusCode >= 400 {
		return w.ResponseWriter.Write(b)
	}
	return w.writer.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	if f, ok := w.writer.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decompress gzip body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(gz)
			defer gz.Close()
		}

		acceptEncoding := r.Header.Get("Accept-Encoding")
		if strings.Contains(acceptEncoding, "gzip") && w.Header().Get("Content-Encoding") == "" {
			w.Header().Set("Vary", "Accept-Encoding")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			gzw := &gzipResponseWriter{ResponseWriter: w, writer: gz, statusCode: http.StatusOK}
			w = gzw
		}

		next.ServeHTTP(w, r)
	})
}
