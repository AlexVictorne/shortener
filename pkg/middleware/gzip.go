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
	io.Writer
	http.ResponseWriter

	hasWrittenHeader bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.hasWrittenHeader {
		if w.Header().Get("Content-Encoding") == "" {
			w.Header().Set("Content-Encoding", "gzip")
		}
		w.hasWrittenHeader = true
	}
	n, err := w.Writer.Write(b)
	if err != nil {
		http.Error(w.ResponseWriter, "Failed to write gzip response", http.StatusInternalServerError)
	}
	return n, err
}

func (w *gzipResponseWriter) Flush() {
	if f, ok := w.Writer.(interface{ Flush() error }); ok {
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
		if strings.Contains(acceptEncoding, "gzip") {
			w.Header().Set("Vary", "Accept-Encoding")
			gz := gzip.NewWriter(w)
			defer func() {
				_ = gz.Close()
			}()
			gzw := &gzipResponseWriter{ResponseWriter: w, Writer: gz}
			w = gzw
		}

		next.ServeHTTP(w, r)
	})
}
