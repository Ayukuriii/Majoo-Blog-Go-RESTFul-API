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

// Envelope is the unified JSON body written by all helpers.
// Unused fields are omitted via omitempty.
type Envelope struct {
	Message *string           `json:"message,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
	Data    any               `json:"data,omitempty"`
	Meta    *Meta             `json:"meta,omitempty"`
	Links   *Links            `json:"links,omitempty"`
}

// DataEnvelope documents WithData / WithStatusData for OpenAPI.
type DataEnvelope struct {
	Message string `json:"message,omitempty"`
	Data    any    `json:"data"`
}

// PaginatedEnvelope documents WithPaginatedData for OpenAPI.
type PaginatedEnvelope struct {
	Message string `json:"message,omitempty"`
	Data    any    `json:"data"`
	Meta    Meta   `json:"meta"`
	Links   Links  `json:"links"`
}

// MessageEnvelope documents WithMessage and Error for OpenAPI.
type MessageEnvelope struct {
	Message string `json:"message"`
}

// ValidationEnvelope documents ValidationError for OpenAPI.
type ValidationEnvelope struct {
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"`
}

// WithData writes a success response with data (default 200).
func WithData(w http.ResponseWriter, data any, message ...string) {
	WithStatusData(w, http.StatusOK, data, message...)
}

// WithStatusData is like WithData but with an explicit HTTP status (e.g. 201).
func WithStatusData(w http.ResponseWriter, status int, data any, message ...string) {
	writeJSON(w, status, Envelope{
		Message: optionalMessage(message),
		Data:    data,
	})
}

// WithPaginatedData writes a 200 response with data, meta, and links.
func WithPaginatedData(w http.ResponseWriter, data any, meta Meta, links Links, message ...string) {
	writeJSON(w, http.StatusOK, Envelope{
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
	writeJSON(w, code, Envelope{Message: &msg})
}

// Error writes a message-only error response (default 400).
func Error(w http.ResponseWriter, message string, status ...int) {
	code := http.StatusBadRequest
	if len(status) > 0 {
		code = status[0]
	}
	msg := message
	writeJSON(w, code, Envelope{Message: &msg})
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
