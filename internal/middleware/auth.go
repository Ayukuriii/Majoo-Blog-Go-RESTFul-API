package middleware

import (
	"blog-api/internal/response"
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey int

const (
	userPublicIDKey contextKey = iota
	requestIDKey
)

// Auth returns middleware that requires a valid Bearer JWT signed with jwtSecret.
// On success it injects the token's `sub` claim (user public_id) into the request context.
func Auth(jwtSecret string) func(http.Handler) http.Handler {
	secret := []byte(jwtSecret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				response.Error(w, "missing or invalid authorization header", http.StatusUnauthorized)
				return
			}

			raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			if raw == "" {
				response.Error(w, "missing or invalid authorization header", http.StatusUnauthorized)
				return
			}

			claims := &jwt.RegisteredClaims{}
			token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
				if t.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrTokenSignatureInvalid
				}
				return secret, nil
			})

			if err != nil || !token.Valid || claims.Subject == "" {
				response.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userPublicIDKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserPublicIDFromContext returns the authenticated user's public_id, if present.
func UserPublicIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userPublicIDKey).(string)
	if !ok || v == "" {
		return "", false
	}

	return v, true
}
