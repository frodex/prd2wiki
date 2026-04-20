package api

import (
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// RequestLogger logs every request with method, path, status, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration", time.Since(start),
		)
	})
}

// RateLimiter limits requests per second.
func RateLimiter(rps float64, burst int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// logMutation emits a structured log line for write operations.
// routeFamily is "project" or "tree"; handler is the handler name (e.g. "createPage").
func logMutation(r *http.Request, routeFamily, handler, project string) {
	hasAuth := apiKeyFromRequest(r) != ""
	slog.Info("mutation",
		"route_family", routeFamily,
		"handler", handler,
		"method", r.Method,
		"project", project,
		"path", r.URL.Path,
		"has_auth", hasAuth,
	)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
