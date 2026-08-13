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
	mux.Handle("GET /api/me", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		me(w, r, s)
	})))
}

func RegisterAuthRoutes(mux *http.ServeMux, s Service) {
	mux.HandleFunc("POST /api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		register(w, r, s)
	})

	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		login(w, r, s)
	})
}

// register registers a new user.
//
//	@Summary		Register a user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		RegisterRequest	true	"Registration payload"
//	@Success		201		{object}	response.DataEnvelope{data=UserResponse}
//	@Failure		400		{object}	response.MessageEnvelope
//	@Failure		409		{object}	response.MessageEnvelope
//	@Failure		422		{object}	response.ValidationEnvelope
//	@Failure		500		{object}	response.MessageEnvelope
//	@Router			/api/auth/register [post]
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

// login authenticates a user and returns a JWT.
//
//	@Summary		Log in
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Login payload"
//	@Success		200		{object}	response.DataEnvelope{data=LoginData}
//	@Failure		400		{object}	response.MessageEnvelope
//	@Failure		401		{object}	response.MessageEnvelope
//	@Failure		404		{object}	response.MessageEnvelope
//	@Failure		422		{object}	response.ValidationEnvelope
//	@Failure		500		{object}	response.MessageEnvelope
//	@Router			/api/auth/login [post]
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

	response.WithData(w, LoginData{
		User:      user,
		Token:     token,
		ExpiresAt: expiresAt,
	}, "Login successful")
}

// me returns the authenticated user.
//
//	@Summary		Get current user
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.DataEnvelope{data=UserResponse}
//	@Failure		401	{object}	response.MessageEnvelope
//	@Failure		404	{object}	response.MessageEnvelope
//	@Failure		500	{object}	response.MessageEnvelope
//	@Router			/api/me [get]
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
