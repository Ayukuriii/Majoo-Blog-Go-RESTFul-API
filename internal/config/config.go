package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port             int
	DBHost           string
	DBPort           int
	DBUser           string
	DBPassword       string
	DBName           string
	JWTSecret        string
	JWTExpiryMinutes int
	Env              string
}

// Load reads configuration from the environment.
// In non-production, it loads a local .env file when present (does not override
// variables already set in the process environment).
func Load() (*Config, error) {
	env := getEnv("APP_ENV", getEnv("ENV", "development"))
	if !isProduction(env) {
		_ = godotenv.Load()
		// Re-read after .env so APP_ENV / ENV from the file take effect.
		env = getEnv("APP_ENV", getEnv("ENV", "development"))
	}

	port, err := atoiEnv("PORT", 8080)
	if err != nil {
		return nil, err
	}

	dbPort, err := atoiEnv("DB_PORT", 3306)
	if err != nil {
		return nil, err
	}

	jwtExpiry, err := atoiEnv("JWT_EXPIRY_MINUTES", 60)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:             port,
		DBHost:           strings.TrimSpace(os.Getenv("DB_HOST")),
		DBPort:           dbPort,
		DBUser:           strings.TrimSpace(os.Getenv("DB_USER")),
		DBPassword:       os.Getenv("DB_PASSWORD"),
		DBName:           strings.TrimSpace(os.Getenv("DB_NAME")),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		JWTExpiryMinutes: jwtExpiry,
		Env:              env,
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Addr returns the HTTP listen address (e.g. ":8080").
func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

// DSN returns a MySQL DSN suitable for GORM's MySQL dialector.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser,
		c.DBPassword,
		c.DBHost,
		c.DBPort,
		c.DBName,
	)
}

func (c *Config) validate() error {
	var missing []string

	if c.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if c.DBHost == "" {
		missing = append(missing, "DB_HOST")
	}
	if c.DBUser == "" {
		missing = append(missing, "DB_USER")
	}
	if c.DBName == "" {
		missing = append(missing, "DB_NAME")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid PORT: %d", c.Port)
	}
	if c.DBPort < 1 || c.DBPort > 65535 {
		return fmt.Errorf("invalid DB_PORT: %d", c.DBPort)
	}
	if c.JWTExpiryMinutes < 1 {
		return fmt.Errorf("invalid JWT_EXPIRY_MINUTES: %d", c.JWTExpiryMinutes)
	}

	return nil
}

func isProduction(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

func atoiEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
