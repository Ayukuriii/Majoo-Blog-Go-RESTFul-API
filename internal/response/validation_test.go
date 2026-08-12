package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
)

type sampleRequest struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=8"`
	Status   string `validate:"omitempty,oneof=active draft"`
}

func TestValidationErrors_Required(t *testing.T) {
	v := validator.New()
	err := v.Struct(sampleRequest{})
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	got := ValidationErrors(ve)
	want := map[string]string{
		"email":    "is required",
		"password": "is required",
	}
	assertStringMap(t, got, want)
}

func TestValidationErrors_EmailAndMinLength(t *testing.T) {
	v := validator.New()
	err := v.Struct(sampleRequest{Email: "not-an-email", Password: "short"})
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	got := ValidationErrors(ve)
	want := map[string]string{
		"email":    "must be a valid email address",
		"password": "must be at least 8 characters",
	}
	assertStringMap(t, got, want)
}

func TestValidationErrors_OneOf(t *testing.T) {
	v := validator.New()
	err := v.Struct(sampleRequest{
		Email:    "a@b.com",
		Password: "password1",
		Status:   "nope",
	})
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	got := ValidationErrors(ve)
	want := map[string]string{
		"status": "must be one of: active, draft",
	}
	assertStringMap(t, got, want)
}

func TestValidationError_Writes422WithErrors(t *testing.T) {
	v := validator.New()
	err := v.Struct(sampleRequest{})
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	rec := httptest.NewRecorder()
	ValidationError(rec, ve)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	body := decodeBody(t, rec)
	assertRawJSON(t, body["message"], `"validation failed"`)

	var errs map[string]string
	if err := json.Unmarshal(body["errors"], &errs); err != nil {
		t.Fatalf("unmarshal errors: %v", err)
	}
	assertStringMap(t, errs, map[string]string{
		"email":    "is required",
		"password": "is required",
	})
	assertAbsent(t, body, "data", "meta", "links")
}

func TestToSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"Email":       "email",
		"DisplayName": "display_name",
		"Password":    "password",
		"":            "",
	} {
		if got := toSnakeCase(in); got != want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func assertStringMap(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("map len = %d, want %d; got %#v", len(got), len(want), got)
	}
	for k, wantV := range want {
		gotV, ok := got[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if gotV != wantV {
			t.Errorf("key %q = %q, want %q", k, gotV, wantV)
		}
	}
}
