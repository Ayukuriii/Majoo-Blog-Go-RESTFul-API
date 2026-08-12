package user

import (
	"blog-api/internal/middleware"
	"blog-api/internal/response"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
)

func RegisterRoutes(mux *http.ServeMux, s Service, auth func(http.Handler) http.Handler) {
	mux.HandleFunc("POST /api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		register(w, r, s)
	})

	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		login(w, r, s)
	})

	mux.Handle("GET /api/me", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		me(w, r, s)
	})))
}

func register(w http.ResponseWriter, r *http.Request, s Service) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	userResp, err := s.Register(r.Context(), req)
	if err != nil {
		var validatorErr validator.ValidationErrors
		switch {
		case errors.As(err, &validatorErr):
			response.ValidationError(w, validatorErr)
		case errors.Is(err, ErrDuplicateEmail):
			response.Error(w, "email already registered", http.StatusConflict) // 409
		default:
			response.Error(w, "failed to register user", http.StatusInternalServerError) // 500
		}
		return
	}

	response.WithStatusData(w, http.StatusCreated, userResp, "User registered successfully")
}

func login(w http.ResponseWriter, r *http.Request, s Service) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	token, expiresAt, err := s.Login(r.Context(), req)
	if err != nil {
		var validatorErr validator.ValidationErrors
		switch {
		case errors.As(err, &validatorErr):
			response.ValidationError(w, validatorErr)
		case errors.Is(err, ErrInvalidCredentials):
			response.Error(w, "invalid email or password", http.StatusUnauthorized) // 401
		default:
			response.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	user, err := s.GetByEmail(r.Context(), req.Email)
	if err != nil {
		response.Error(w, "user not found", http.StatusNotFound) // 404
		return
	}

	response.WithData(w, map[string]any{
		"user":       user,
		"token":      token,
		"expires_at": expiresAt,
	}, "Login successful")
}

func me(w http.ResponseWriter, r *http.Request, s Service) {
	publicID, ok := middleware.UserPublicIDFromContext(r.Context())
	if !ok {
		response.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userResp, err := s.GetByPublicID(r.Context(), publicID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, "user not found", http.StatusNotFound)
			return
		}

		response.Error(w, "failed to get user", http.StatusInternalServerError)
		return
	}

	response.WithData(w, userResp, "Current user retrieved")
}
