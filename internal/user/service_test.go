package user

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"
)

type mockRepo struct {
	createFn        func(ctx context.Context, user *User) error
	getByEmailFn    func(ctx context.Context, email string) (*User, error)
	getByPublicIDFn func(ctx context.Context, publicID string) (*User, error)
}

func (m *mockRepo) Create(ctx context.Context, user *User) error {
	return m.createFn(ctx, user)
}
func (m *mockRepo) GetByID(context.Context, uint64) (*User, error) {
	return nil, errors.New("not used")
}
func (m *mockRepo) GetByPublicID(ctx context.Context, publicID string) (*User, error) {
	return m.getByPublicIDFn(ctx, publicID)
}
func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	return m.getByEmailFn(ctx, email)
}

func newTestService(repo Repository) Service {
	return NewService(repo, validator.New(), "test-secret", 60, bcrypt.MinCost)
}

func TestRegister_Valid(t *testing.T) {
	var created *User
	repo := &mockRepo{
		createFn: func(_ context.Context, u *User) error {
			created = u
			u.CreatedAt = time.Now()
			u.UpdatedAt = u.CreatedAt
			return nil
		},
	}
	svc := newTestService(repo)

	res, err := svc.Register(context.Background(), RegisterRequest{
		Email:       "a@example.com",
		Password:    "password1",
		DisplayName: "Ada",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.PublicID == "" || res.Email != "a@example.com" || res.DisplayName == nil || *res.DisplayName != "Ada" {
		t.Fatalf("unexpected response: %+v", res)
	}
	if created == nil || created.PasswordHash == "" || created.PasswordHash == "password1" {
		t.Fatal("password must be hashed before Create")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("password1")); err != nil {
		t.Fatalf("hash does not match password: %v", err)
	}
	// UUID v7 string form is 36 chars with hyphens
	if len(created.PublicID) != 36 {
		t.Fatalf("public_id length: %q", created.PublicID)
	}
}

func TestRegister_InvalidPayload(t *testing.T) {
	svc := newTestService(&mockRepo{})
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "not-an-email",
		Password: "short",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := &mockRepo{
		createFn: func(context.Context, *User) error { return ErrDuplicateEmail },
	}
	svc := newTestService(repo)
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "a@example.com",
		Password: "password1",
	})
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("want ErrDuplicateEmail, got %v", err)
	}
}

func TestLogin_Success_JWTSubIsPublicID(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	const publicID = "0190f0e2-8c3a-7b2d-9e4f-1a2b3c4d5e6f"
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{
				ID:           99, // must NOT appear in JWT
				PublicID:     publicID,
				Email:        "a@example.com",
				PasswordHash: string(hash),
			}, nil
		},
	}
	svc := newTestService(repo)

	token, expiresAt, err := svc.Login(context.Background(), LoginRequest{
		Email:    "a@example.com",
		Password: "password1",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" || expiresAt.Before(time.Now()) {
		t.Fatalf("bad token/expiry: %q %v", token, expiresAt)
	}

	sub := jwtSubUnverified(t, token)
	if sub != publicID {
		t.Fatalf("sub=%q, want public_id %q", sub, publicID)
	}
	if sub == "99" {
		t.Fatal("sub must not be internal id")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "x", Email: "a@example.com", PasswordHash: string(hash)}, nil
		},
	}
	svc := newTestService(repo)
	_, _, err := svc.Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return nil, ErrNotFound
		},
	}
	svc := newTestService(repo)
	_, _, err := svc.Login(context.Background(), LoginRequest{
		Email: "missing@example.com", Password: "password1",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials (not ErrNotFound), got %v", err)
	}
}

// jwtSubUnverified base64-decodes the JWT payload (no signature check) for unit tests.
func jwtSubUnverified(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("json: %v", err)
	}
	sub, _ := claims["sub"].(string)
	return sub
}
