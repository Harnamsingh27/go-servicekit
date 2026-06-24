package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"go-servicekit/observability"
)

// Chain wraps handler with middlewares applied outermost-first.
// The first middleware in the slice is the outermost (runs first on a request,
// last on a response).
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// RequestIDMiddleware reuses the X-Request-ID header if present, otherwise
// generates a new ID. It stores the ID in context and echoes it back in the
// response.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = observability.NewRequestID()
		}
		ctx := observability.WithRequestID(r.Context(), rid)
		w.Header().Set("X-Request-ID", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs each request's method, path, status code, and latency
// using the structured logger. It uses the logger attached to the request
// context if present, otherwise falls back to the slog default.
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.code),
				slog.Duration("latency", time.Since(start)),
				slog.String("request_id", observability.RequestIDFromContext(r.Context())),
			)
		})
	}
}

// statusWriter captures the HTTP status code so it can be logged after the
// handler writes the response.
type statusWriter struct {
	http.ResponseWriter
	code    int
	written bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.written {
		sw.code = code
		sw.written = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
