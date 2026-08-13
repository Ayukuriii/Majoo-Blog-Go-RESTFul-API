package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"blog-api/internal/response"
)

// Recovery catches panics, logs them, and responds with 500 instead of crashing the process.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r, reqID := withRequestID(r)
			w.Header().Set(requestIDHeader, reqID)

			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				logger.Error("panic recovered",
					"panic", fmt.Sprint(rec),
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", reqID,
				)
				response.Error(w, "internal server error", http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
