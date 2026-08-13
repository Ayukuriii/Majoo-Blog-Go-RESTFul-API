// @title			Majoo Blog RESTful API
// @version		1.0
// @description	Blog platform API (users, posts, comments). Public identifiers are UUID v7 public_id values; raw database ids are never exposed.
// @host			localhost:8080
// @BasePath		/
//
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				JWT. Swagger UI uses HTTP bearer (paste the token only).
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"blog-api/docs/swagger"
	"blog-api/internal/comment"
	"blog-api/internal/config"
	"blog-api/internal/database"
	"blog-api/internal/middleware"
	"blog-api/internal/post"
	"blog-api/internal/user"

	"github.com/go-playground/validator/v10"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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

	commentRepo := comment.NewRepository(db)
	commentService := comment.NewService(commentRepo, postRepo, userRepo, db, validate)

	mux := http.NewServeMux()
	// Exact path must win over /swagger/ so http-swagger does not call swag.ReadDoc
	// (generated docs register swag/v2; http-swagger/v2 reads swag v1).
	mux.HandleFunc("GET /swagger/doc.json", swagger.ServeSpec)
	mux.Handle("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
	))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	if !cfg.IsProduction() {
		mux.HandleFunc("GET /api/debug/panic", func(http.ResponseWriter, *http.Request) {
			panic("deliberate panic")
		})
	}

	auth := middleware.Auth(cfg.JWTSecret)

	authMux := http.NewServeMux()
	user.RegisterAuthRoutes(authMux, userService)
	mux.Handle("/api/auth/", middleware.RateLimitByIP(5)(authMux))

	user.RegisterRoutes(mux, userService, auth)
	post.RegisterRoutes(mux, postService, auth)
	comment.RegisterRoutes(mux, commentService, auth)

	handler := middleware.Chain(mux,
		middleware.Recovery(logger),
		middleware.Logging(logger),
		middleware.CORS(cfg.CORSAllowedOrigins),
	)

	addr := cfg.Addr()
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
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
