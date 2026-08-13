package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Logging records method, path, status, latency, and request id as structured slog fields.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r, reqID := withRequestID(r)
			w.Header().Set(requestIDHeader, reqID)

			statusWriter := &statusWriter{ResponseWriter: w}
			start := time.Now()
			defer func() {
				status := statusWriter.status
				if rec := recover(); rec != nil {
					if status == 0 {
						status = http.StatusInternalServerError
					}
					logger.Info("http request",
						"method", r.Method,
						"path", r.URL.Path,
						"status", status,
						"latency_ms", time.Since(start).Milliseconds(),
						"request_id", reqID,
					)
					panic(rec)
				}
				if status == 0 {
					status = http.StatusOK
				}
				logger.Info("http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"latency_ms", time.Since(start).Milliseconds(),
					"request_id", reqID,
				)
			}()

			next.ServeHTTP(statusWriter, r)
		})
	}
}
