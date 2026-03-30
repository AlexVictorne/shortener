package middleware

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (w *LoggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *LoggingResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *LoggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *LoggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *LoggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

// RequestResponseLogger logs request and response info as separate messages.
func RequestResponseLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &LoggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		status := lrw.statusCode
		if status == 0 {
			status = http.StatusOK
		}

		// Log request info: URI, method, duration
		log.Info().
			Str("uri", r.RequestURI).
			Str("method", r.Method).
			Dur("duration", duration).
			Msg("request")

		// Log response info: status, size
		log.Info().
			Int("status", status).
			Int("size", lrw.bytes).
			Msg("response")
	})
}
