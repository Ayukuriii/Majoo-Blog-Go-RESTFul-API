package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type mockRepo struct {
	createFn        func(ctx context.Context, user *User) error
	getByEmailFn    func(ctx context.Context, email string) (*User, error)
	getByPublicIDFn func(ctx context.Context, publicID string) (*User, error)
}

func (m *mockRepo) WithTx(*gorm.DB) Repository { return m }
func (m *mockRepo) Create(ctx context.Context, user *User) error {
	return m.createFn(ctx, user)
}
func (m *mockRepo) GetByID(context.Context, uint64) (*User, error) {
	return nil, errors.New("not used")
}
func (m *mockRepo) GetByIDs(context.Context, []uint64) ([]User, error) {
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
	require.NoError(t, err)
	require.NotEmpty(t, res.PublicID)
	assert.Equal(t, "a@example.com", res.Email)
	require.NotNil(t, res.DisplayName)
	assert.Equal(t, "Ada", *res.DisplayName)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.PasswordHash)
	assert.NotEqual(t, "password1", created.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("password1")))
	assert.Len(t, created.PublicID, 36)
}

func TestRegister_InvalidPayload(t *testing.T) {
	svc := newTestService(&mockRepo{})
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "not-an-email",
		Password: "short",
	})
	require.Error(t, err)
}

func TestRegister_PasswordTooShort(t *testing.T) {
	svc := newTestService(&mockRepo{})
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "a@example.com",
		Password: "1234567",
	})
	require.Error(t, err)
}

func TestRegister_EmptyPassword(t *testing.T) {
	svc := newTestService(&mockRepo{})
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "a@example.com",
		Password: "",
	})
	require.Error(t, err)
}

func TestRegister_PasswordAtBcryptLimit(t *testing.T) {
	var created *User
	repo := &mockRepo{
		createFn: func(_ context.Context, u *User) error {
			created = u
			return nil
		},
	}
	svc := newTestService(repo)
	password := strings.Repeat("a", 72)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "a@example.com",
		Password: password,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte(password)))
}

func TestRegister_PasswordExceedsBcryptLimit(t *testing.T) {
	svc := newTestService(&mockRepo{createFn: func(context.Context, *User) error {
		t.Fatal("Create must not run when hashing fails")
		return nil
	}})
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "a@example.com",
		Password: strings.Repeat("a", 73),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
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
	assert.ErrorIs(t, err, ErrDuplicateEmail)
}

func TestLogin_Success_JWTSubIsPublicID(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	require.NoError(t, err)
	const publicID = "0190f0e2-8c3a-7b2d-9e4f-1a2b3c4d5e6f"
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{
				ID:           99,
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
	require.NoError(t, err)
	require.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now()))

	claims := parseValidToken(t, token, "test-secret")
	assert.Equal(t, publicID, claims.Subject)
	assert.NotEqual(t, "99", claims.Subject)
	require.NotNil(t, claims.ExpiresAt)
	assert.WithinDuration(t, expiresAt, claims.ExpiresAt.Time, time.Second)
}

func TestLogin_JWTRejectsTamperedToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	require.NoError(t, err)
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "user-1", Email: "a@example.com", PasswordHash: string(hash)}, nil
		},
	}
	token, _, err := newTestService(repo).Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "password1",
	})
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	tampered := parts[0] + "." + parts[1] + ".AAAA"
	_, err = jwt.Parse(tampered, func(tkn *jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	require.Error(t, err)
}

func TestLogin_JWTRejectsExpiredToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	require.NoError(t, err)
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "user-1", Email: "a@example.com", PasswordHash: string(hash)}, nil
		},
	}
	svc := NewService(repo, validator.New(), "test-secret", -1, bcrypt.MinCost)
	token, _, err := svc.Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "password1",
	})
	require.NoError(t, err)

	_, err = jwt.Parse(token, func(tkn *jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestLogin_JWTRejectsWrongSecret(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	require.NoError(t, err)
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "user-1", Email: "a@example.com", PasswordHash: string(hash)}, nil
		},
	}
	token, _, err := newTestService(repo).Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "password1",
	})
	require.NoError(t, err)

	_, err = jwt.Parse(token, func(tkn *jwt.Token) (any, error) {
		return []byte("other-secret"), nil
	})
	require.Error(t, err)
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	require.NoError(t, err)
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "x", Email: "a@example.com", PasswordHash: string(hash)}, nil
		},
	}
	svc := newTestService(repo)
	_, _, err = svc.Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "wrong-password",
	})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_CorruptPasswordHash(t *testing.T) {
	repo := &mockRepo{
		getByEmailFn: func(context.Context, string) (*User, error) {
			return &User{PublicID: "x", Email: "a@example.com", PasswordHash: "not-a-bcrypt-hash"}, nil
		},
	}
	_, _, err := newTestService(repo).Login(context.Background(), LoginRequest{
		Email: "a@example.com", Password: "password1",
	})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
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
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestGetByPublicID_UnknownVsMalformed(t *testing.T) {
	const known = "0190f0e2-8c3a-7b2d-9e4f-1a2b3c4d5e6f"
	repo := &mockRepo{
		getByPublicIDFn: func(_ context.Context, publicID string) (*User, error) {
			if publicID == known {
				return &User{PublicID: known, Email: "a@example.com"}, nil
			}
			return nil, ErrNotFound
		},
	}
	svc := newTestService(repo)

	got, err := svc.GetByPublicID(context.Background(), known)
	require.NoError(t, err)
	assert.Equal(t, known, got.PublicID)

	_, err = svc.GetByPublicID(context.Background(), "0190f0e2-8c3a-7b2d-9e4f-ffffffffffff")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = svc.GetByPublicID(context.Background(), "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestGetByEmail(t *testing.T) {
	repo := &mockRepo{
		getByEmailFn: func(_ context.Context, email string) (*User, error) {
			if email == "a@example.com" {
				return &User{PublicID: "user-1", Email: email, PasswordHash: "secret"}, nil
			}
			return nil, ErrNotFound
		},
	}
	svc := newTestService(repo)

	got, err := svc.GetByEmail(context.Background(), "a@example.com")
	require.NoError(t, err)
	assert.Equal(t, "user-1", got.PublicID)
	assert.Equal(t, "a@example.com", got.Email)

	_, err = svc.GetByEmail(context.Background(), "missing@example.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func parseValidToken(t *testing.T, token, secret string) *jwt.RegisteredClaims {
	t.Helper()
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(tkn *jwt.Token) (any, error) {
		require.Equal(t, jwt.SigningMethodHS256, tkn.Method)
		return []byte(secret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	return claims
}
