package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"blog-api/internal/response"
)

type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// RateLimitByIP is an in-memory token bucket limiter (capacity = ratePerMinute, refill over 1 minute).
func RateLimitByIP(ratePerMinute int) func(http.Handler) http.Handler {
	if ratePerMinute < 1 {
		ratePerMinute = 5
	}
	capacity := float64(ratePerMinute)
	refillPerSec := capacity / 60.0

	var mu sync.Mutex
	buckets := make(map[string]*tokenBucket)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-10 * time.Minute)
			mu.Lock()
			for ip, b := range buckets {
				if b.lastSeen.Before(cutoff) {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			now := time.Now()

			mu.Lock()
			b, ok := buckets[ip]
			if !ok {
				b = &tokenBucket{tokens: capacity, last: now, lastSeen: now}
				buckets[ip] = b
			}
			elapsed := now.Sub(b.last).Seconds()
			b.tokens += elapsed * refillPerSec
			if b.tokens > capacity {
				b.tokens = capacity
			}
			b.last = now
			b.lastSeen = now
			allowed := b.tokens >= 1
			if allowed {
				b.tokens--
			}
			mu.Unlock()

			if !allowed {
				w.Header().Set("Retry-After", "60")
				response.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if r.RemoteAddr != "" {
			return r.RemoteAddr
		}
		return "unknown"
	}
	return host
}
