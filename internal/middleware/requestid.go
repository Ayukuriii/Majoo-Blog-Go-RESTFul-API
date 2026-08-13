package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// RequestIDFromContext returns the request id set by Recovery/Logging middleware.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func withRequestID(r *http.Request) (*http.Request, string) {
	id := RequestIDFromContext(r.Context())
	if id == "" {
		id = r.Header.Get(requestIDHeader)
	}
	if id == "" {
		if u, err := uuid.NewV7(); err == nil {
			id = u.String()
		} else {
			id = uuid.NewString()
		}
	}
	ctx := context.WithValue(r.Context(), requestIDKey, id)
	return r.WithContext(ctx), id
}
