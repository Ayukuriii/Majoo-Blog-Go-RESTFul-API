package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	HTTPAddr string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	JWTSecret string
}

// Load reads configuration from the environment.
// Callers may load a .env file before invoking Load if desired.
func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("DB_PORT", "3306"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	cfg := &Config{
		HTTPAddr:   getEnv("HTTP_ADDR", ":8080"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     port,
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "blog"),
		JWTSecret:  getEnv("JWT_SECRET", ""),
	}

	return cfg, nil
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

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
