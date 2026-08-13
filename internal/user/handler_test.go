//go:build integration

package user_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"blog-api/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emailFor(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s@example.com", strings.ToLower(strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())))
}

func TestIntegration_RegisterLoginMe(t *testing.T) {
	s := testutil.NewServer(t)
	email := emailFor(t)
	const password = "secret123"

	reg := s.DoJSON(t, http.MethodPost, "/api/auth/register", "", map[string]any{
		"email":        email,
		"password":     password,
		"display_name": "Ada",
	}, http.StatusCreated)
	assert.Equal(t, "User registered successfully", reg["message"])
	user := testutil.DataObject(t, reg)
	assert.Equal(t, email, user["email"])
	assert.NotEmpty(t, user["public_id"])
	_, hasHash := user["password_hash"]
	assert.False(t, hasHash)

	login := s.DoJSON(t, http.MethodPost, "/api/auth/login", "", map[string]any{
		"email":    email,
		"password": password,
	}, http.StatusOK)
	assert.Equal(t, "Login successful", login["message"])
	data := testutil.DataObject(t, login)
	token, _ := data["token"].(string)
	require.NotEmpty(t, token)
	assert.NotEmpty(t, data["expires_at"])
	logged := data["user"].(map[string]any)
	assert.Equal(t, user["public_id"], logged["public_id"])

	me := s.DoJSON(t, http.MethodGet, "/api/me", token, nil, http.StatusOK)
	assert.Equal(t, "Current user retrieved", me["message"])
	assert.Equal(t, user["public_id"], testutil.DataObject(t, me)["public_id"])
}

func TestIntegration_UnauthorizedMissingToken(t *testing.T) {
	s := testutil.NewServer(t)
	env := s.DoJSON(t, http.MethodGet, "/api/me", "", nil, http.StatusUnauthorized)
	assert.Equal(t, "missing or invalid authorization header", env["message"])
}

func TestIntegration_LoginInvalidCredentials(t *testing.T) {
	s := testutil.NewServer(t)
	email := emailFor(t)
	s.RegisterUser(t, email, "secret123", "")
	env := s.DoJSON(t, http.MethodPost, "/api/auth/login", "", map[string]any{
		"email":    email,
		"password": "wrong-password",
	}, http.StatusUnauthorized)
	assert.Equal(t, "invalid email or password", env["message"])
}

func TestIntegration_RegisterValidationError(t *testing.T) {
	s := testutil.NewServer(t)
	env := s.DoJSON(t, http.MethodPost, "/api/auth/register", "", map[string]any{}, http.StatusUnprocessableEntity)
	assert.Equal(t, "validation failed", env["message"])
	errs := env["errors"].(map[string]any)
	assert.NotEmpty(t, errs["email"])
	assert.NotEmpty(t, errs["password"])
}
