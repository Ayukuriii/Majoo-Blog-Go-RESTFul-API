package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func TestRecovery_PanicReturns500AndKeepsServing(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) {
		panic("deliberate panic")
	})

	h := Chain(mux, Recovery(logger), Logging(logger))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", rec.Code)
	}
	if rec.Header().Get(requestIDHeader) == "" {
		t.Fatal("missing X-Request-ID on panic response")
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode panic body: %v", err)
	}
	if body["message"] != "internal server error" {
		t.Fatalf("message = %q", body["message"])
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("server crashed or misbehaved after panic: status = %d", rec2.Code)
	}

	if !strings.Contains(buf.String(), `"request_id"`) {
		t.Fatalf("logs missing request_id: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"panic recovered"`) && !strings.Contains(buf.String(), `"deliberate panic"`) {
		t.Fatalf("logs missing panic: %s", buf.String())
	}
}

func TestLogging_StructuredJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.Header.Set(requestIDHeader, "req-fixed-id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log json: %v\n%s", err, buf.String())
	}
	if entry["msg"] != "http request" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if entry["method"] != "POST" {
		t.Errorf("method = %v", entry["method"])
	}
	if entry["path"] != "/api/items" {
		t.Errorf("path = %v", entry["path"])
	}
	if entry["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v", entry["status"])
	}
	if _, ok := entry["latency_ms"]; !ok {
		t.Error("missing latency_ms")
	}
	if entry["request_id"] != "req-fixed-id" {
		t.Errorf("request_id = %v", entry["request_id"])
	}
}

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	h := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	h := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected Allow-Origin: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestRateLimitByIP_Returns429(t *testing.T) {
	h := RateLimitByIP(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqFor := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "203.0.113.10:54321"
		return req
	}

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqFor())
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqFor())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["message"] != "too many requests" {
		t.Fatalf("message = %q", body["message"])
	}
}

func TestRateLimitByIP_DoesNotApplyOutsideAuthMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	authMux := http.NewServeMux()
	authMux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/api/auth/", RateLimitByIP(5)(authMux))

	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "203.0.113.10:54321"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("health request %d status = %d", i+1, rec.Code)
		}
	}
}
