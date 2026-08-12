package response

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// Meta describes pagination metadata for list endpoints.
type Meta struct {
	CurrentPage int            `json:"current_page"`
	From        *int           `json:"from"`
	LastPage    int            `json:"last_page"`
	Path        string         `json:"path"`
	PerPage     int            `json:"per_page"`
	To          *int           `json:"to"`
	Total       int64          `json:"total"`
	Search      string         `json:"search,omitempty"`
	Sort        string         `json:"sort,omitempty"`
	Order       string         `json:"order,omitempty"` // asc | desc
	Filters     map[string]any `json:"filters,omitempty"`
}

// Links holds pagination navigation URLs.
type Links struct {
	First string  `json:"first"`
	Last  string  `json:"last"`
	Prev  *string `json:"prev"`
	Next  *string `json:"next"`
}

type envelope struct {
	Message *string `json:"message,omitempty"`
	Data    any     `json:"data,omitempty"`
	Meta    *Meta   `json:"meta,omitempty"`
	Links   *Links  `json:"links,omitempty"`
}

// WithData writes a 200 response with optional message and data payload.
func WithData(w http.ResponseWriter, data any, message ...string) {
	writeJSON(w, http.StatusOK, envelope{
		Message: optionalMessage(message),
		Data:    data,
	})
}

// WithPaginatedData writes a 200 response with data, meta, and links.
func WithPaginatedData(w http.ResponseWriter, data any, meta Meta, links Links, message ...string) {
	writeJSON(w, http.StatusOK, envelope{
		Message: optionalMessage(message),
		Data:    data,
		Meta:    &meta,
		Links:   &links,
	})
}

// WithMessage writes a message-only success response (default 200).
func WithMessage(w http.ResponseWriter, message string, status ...int) {
	code := http.StatusOK
	if len(status) > 0 {
		code = status[0]
	}
	msg := message
	writeJSON(w, code, envelope{Message: &msg})
}

// Error writes a message-only error response (default 400).
func Error(w http.ResponseWriter, message string, status ...int) {
	code := http.StatusBadRequest
	if len(status) > 0 {
		code = status[0]
	}
	msg := message
	writeJSON(w, code, envelope{Message: &msg})
}

func optionalMessage(message []string) *string {
	if len(message) == 0 || message[0] == "" {
		return nil
	}
	msg := message[0]
	return &msg
}

// writeJSON encodes with flags matching JSON_UNESCAPED_UNICODE |
// JSON_UNESCAPED_SLASHES | JSON_PRESERVE_ZERO_FRACTION.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"message":"failed to encode response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}
