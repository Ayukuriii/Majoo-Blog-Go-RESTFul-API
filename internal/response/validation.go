package response

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// ValidationError writes a 422 response with a summary message and per-field errors.
//
//	{
//	  "message": "validation failed",
//	  "errors": { "email": "is required", "password": "is required" }
//	}
func ValidationError(w http.ResponseWriter, errs validator.ValidationErrors) {
	msg := "validation failed"
	writeJSON(w, http.StatusUnprocessableEntity, Envelope{
		Message: &msg,
		Errors:  ValidationErrors(errs),
	})
}

// ValidationErrors maps field names (snake_case) to short human-readable reasons.
func ValidationErrors(errs validator.ValidationErrors) map[string]string {
	out := make(map[string]string, len(errs))
	for _, e := range errs {
		field := toSnakeCase(e.Field())
		// First failure per field wins (validator typically reports one tag per field).
		if _, exists := out[field]; !exists {
			out[field] = fieldErrorReason(e)
		}
	}
	return out
}

func fieldErrorReason(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		if e.Kind() == reflect.String {
			return fmt.Sprintf("must be at least %s characters", e.Param())
		}
		return fmt.Sprintf("must be at least %s", e.Param())
	case "max":
		if e.Kind() == reflect.String {
			return fmt.Sprintf("must be at most %s characters", e.Param())
		}
		return fmt.Sprintf("must be at most %s", e.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", strings.ReplaceAll(e.Param(), " ", ", "))
	default:
		return "is invalid"
	}
}

func toSnakeCase(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
