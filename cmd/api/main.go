package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"blog-api/internal/config"
	"blog-api/internal/database"
	"blog-api/internal/middleware"
	"blog-api/internal/post"
	"blog-api/internal/user"

	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Use a temporary JSON logger so startup failures are still structured.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("config load failed", "error", err)
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if cfg.Env == "development" || cfg.Env == "dev" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	db, err := database.Open(cfg.DSN())
	if err != nil {
		logger.Error("database open failed", "error", err)
		os.Exit(1)
	}
	validate := validator.New()

	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, validate, cfg.JWTSecret, cfg.JWTExpiryMinutes, bcrypt.DefaultCost)

	postRepo := post.NewRepository(db)
	postService := post.NewService(postRepo, userRepo, db, validate)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	auth := middleware.Auth(cfg.JWTSecret)

	user.RegisterRoutes(mux, userService, auth)
	post.RegisterRoutes(mux, postService, auth)

	addr := cfg.Addr()
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("server starting",
		"addr", addr,
		"env", cfg.Env,
	)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
