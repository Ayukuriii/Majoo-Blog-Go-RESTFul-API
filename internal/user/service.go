package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// Service is the user business-logic API.
type Service interface {
	Register(ctx context.Context, req RegisterRequest) (UserResponse, error)
	Login(ctx context.Context, req LoginRequest) (token string, expiresAt time.Time, err error)
	GetByPublicID(ctx context.Context, publicID string) (UserResponse, error)
	GetByEmail(ctx context.Context, email string) (UserResponse, error)
}

type service struct {
	repo             Repository
	validate         *validator.Validate
	jwtSecret        []byte
	jwtExpiryMinutes int
	bcryptCost       int
}

// NewService wires a user Service. bcryptCost is typically bcrypt.DefaultCost (or from config later).
func NewService(repo Repository, validate *validator.Validate, jwtSecret string, jwtExpiryMinutes, bcryptCost int) Service {
	return &service{
		repo:             repo,
		validate:         validate,
		jwtSecret:        []byte(jwtSecret),
		jwtExpiryMinutes: jwtExpiryMinutes,
		bcryptCost:       bcryptCost,
	}
}

func (s *service) Register(ctx context.Context, req RegisterRequest) (UserResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return UserResponse{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return UserResponse{}, fmt.Errorf("hash password: %w", err)
	}

	publicID, err := uuid.NewV7()
	if err != nil {
		return UserResponse{}, fmt.Errorf("generate public ID: %w", err)
	}

	u := &User{
		PublicID:     publicID.String(),
		Email:        req.Email,
		PasswordHash: string(hash),
	}

	if req.DisplayName != "" {
		name := req.DisplayName
		u.DisplayName = &name
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return UserResponse{}, err
	}

	return toUserResponse(u), nil
}

func (s *service) Login(ctx context.Context, req LoginRequest) (string, time.Time, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", time.Time{}, err
	}

	u, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", time.Time{}, ErrInvalidCredentials
		}
		return "", time.Time{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}

	expiresAt := time.Now().Add(time.Duration(s.jwtExpiryMinutes) * time.Minute)
	claims := jwt.RegisteredClaims{
		Subject:   u.PublicID,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}

	return signed, expiresAt, nil
}

func (s *service) GetByPublicID(ctx context.Context, publicID string) (UserResponse, error) {
	u, err := s.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return UserResponse{}, err
	}

	return toUserResponse(u), nil
}

func (s *service) GetByEmail(ctx context.Context, email string) (UserResponse, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return UserResponse{}, err
	}
	return toUserResponse(u), nil
}

func toUserResponse(u *User) UserResponse {
	return UserResponse{
		PublicID:    u.PublicID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
