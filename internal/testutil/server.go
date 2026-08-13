//go:build integration

package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"blog-api/internal/comment"
	"blog-api/internal/middleware"
	"blog-api/internal/post"
	"blog-api/internal/user"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const JWTSecret = "integration-test-jwt-secret"

// Server is a fully wired API (handlers + auth + GORM MySQL) for httptest.
type Server struct {
	DB      *gorm.DB
	Handler http.Handler
}

func NewServer(t *testing.T) *Server {
	t.Helper()
	db := openSharedDB(t)
	truncateAll(t, db)
	t.Cleanup(func() { truncateAll(t, db) })

	validate := validator.New()
	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, validate, JWTSecret, 60, bcrypt.MinCost)
	postRepo := post.NewRepository(db)
	postService := post.NewService(postRepo, userRepo, db, validate)
	commentRepo := comment.NewRepository(db)
	commentService := comment.NewService(commentRepo, postRepo, userRepo, db, validate)

	mux := http.NewServeMux()
	auth := middleware.Auth(JWTSecret)
	user.RegisterAuthRoutes(mux, userService)
	user.RegisterRoutes(mux, userService, auth)
	post.RegisterRoutes(mux, postService, auth)
	comment.RegisterRoutes(mux, commentService, auth)

	return &Server{DB: db, Handler: mux}
}

func (s *Server) Do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler.ServeHTTP(rec, req)
	return rec
}

func (s *Server) DoJSON(t *testing.T, method, path, token string, body any, wantStatus int) map[string]any {
	t.Helper()
	rec := s.Do(t, method, path, token, body)
	require.Equal(t, wantStatus, rec.Code, "body: %s", rec.Body.String())
	return AssertEnvelope(t, rec)
}

type AuthUser struct {
	Email    string
	Password string
	Token    string
	PublicID string
}

func (s *Server) RegisterUser(t *testing.T, email, password, displayName string) string {
	t.Helper()
	body := map[string]any{"email": email, "password": password}
	if displayName != "" {
		body["display_name"] = displayName
	}
	env := s.DoJSON(t, http.MethodPost, "/api/auth/register", "", body, http.StatusCreated)
	data, _ := env["data"].(map[string]any)
	id, _ := data["public_id"].(string)
	require.NotEmpty(t, id)
	return id
}

func (s *Server) LoginUser(t *testing.T, email, password string) AuthUser {
	t.Helper()
	env := s.DoJSON(t, http.MethodPost, "/api/auth/login", "", map[string]any{
		"email":    email,
		"password": password,
	}, http.StatusOK)
	data, _ := env["data"].(map[string]any)
	require.NotNil(t, data)
	token, _ := data["token"].(string)
	userObj, _ := data["user"].(map[string]any)
	require.NotEmpty(t, token)
	require.NotNil(t, userObj)
	id, _ := userObj["public_id"].(string)
	require.NotEmpty(t, id)
	return AuthUser{Email: email, Password: password, Token: token, PublicID: id}
}

func (s *Server) RegisterAndLogin(t *testing.T, email, password, displayName string) AuthUser {
	t.Helper()
	s.RegisterUser(t, email, password, displayName)
	return s.LoginUser(t, email, password)
}
