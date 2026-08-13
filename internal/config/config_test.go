package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_MissingRequired(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production") // skip .env file

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required env vars")
	}
	msg := err.Error()
	for _, key := range []string{"JWT_SECRET", "DB_HOST", "DB_USER", "DB_NAME"} {
		if !strings.Contains(msg, key) {
			t.Errorf("error %q should mention %s", msg, key)
		}
	}
}

func TestLoad_Success(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "9090")
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USER", "blog")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "blog")
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("JWT_EXPIRY_MINUTES", "120")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, https://blog.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.Addr() != ":9090" {
		t.Errorf("Addr() = %q, want :9090", cfg.Addr())
	}
	if cfg.DBHost != "db.example" || cfg.DBPort != 3307 || cfg.DBUser != "blog" || cfg.DBName != "blog" {
		t.Errorf("unexpected DB fields: %+v", cfg)
	}
	if cfg.JWTSecret != "super-secret" || cfg.JWTExpiryMinutes != 120 {
		t.Errorf("unexpected JWT fields: %+v", cfg)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want production", cfg.Env)
	}
	if len(cfg.CORSAllowedOrigins) != 2 ||
		cfg.CORSAllowedOrigins[0] != "http://localhost:3000" ||
		cfg.CORSAllowedOrigins[1] != "https://blog.example" {
		t.Errorf("CORSAllowedOrigins = %#v", cfg.CORSAllowedOrigins)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"APP_ENV", "ENV", "PORT",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"JWT_SECRET", "JWT_EXPIRY_MINUTES", "CORS_ALLOWED_ORIGINS",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}
