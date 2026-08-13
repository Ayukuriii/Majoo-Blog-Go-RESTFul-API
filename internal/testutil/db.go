//go:build integration

package testutil

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultTestDBName = "blog_test"

var identRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type dbConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

var (
	dbOnce sync.Once
	gormDB *gorm.DB
	dbErr  error
)

func loadDBConfig() (dbConfig, error) {
	host := firstEnv("TEST_DB_HOST", "DB_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	user := firstEnv("TEST_DB_USER", "DB_USER")
	if user == "" {
		user = "root"
	}
	pass := firstEnv("TEST_DB_PASSWORD", "DB_PASSWORD")
	name := os.Getenv("TEST_DB_NAME")
	if strings.TrimSpace(name) == "" {
		name = defaultTestDBName
	}
	if !identRE.MatchString(name) {
		return dbConfig{}, fmt.Errorf("TEST_DB_NAME %q is not a valid schema name", name)
	}

	portStr := firstEnv("TEST_DB_PORT", "DB_PORT")
	port := 3306
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return dbConfig{}, fmt.Errorf("invalid TEST_DB_PORT/DB_PORT: %q", portStr)
		}
		port = p
	}

	return dbConfig{Host: host, Port: port, User: user, Password: pass, Name: name}, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (c dbConfig) serverDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&multiStatements=true",
		c.User, c.Password, c.Host, c.Port)
}

func (c dbConfig) schemaDSN(extra string) string {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Name)
	if extra != "" {
		dsn += "&" + extra
	}
	return dsn
}

func openSharedDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbOnce.Do(func() {
		gormDB, dbErr = initTestMySQL()
	})
	if dbErr != nil {
		t.Fatalf("integration MySQL: %v\nSet TEST_DB_* (see TESTING.md). Example: TEST_DB_HOST=127.0.0.1 TEST_DB_PORT=3306 TEST_DB_USER=root TEST_DB_NAME=blog_test", dbErr)
	}
	return gormDB
}

func initTestMySQL() (*gorm.DB, error) {
	cfg, err := loadDBConfig()
	if err != nil {
		return nil, err
	}

	admin, err := sql.Open("mysql", cfg.serverDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql server: %w", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql server (%s:%d): %w", cfg.Host, cfg.Port, err)
	}
	_, err = admin.Exec("CREATE DATABASE IF NOT EXISTS `" + cfg.Name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if err != nil {
		return nil, fmt.Errorf("create schema %s: %w", cfg.Name, err)
	}

	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	if err := runMigrations(cfg, filepath.Join(root, "migrations")); err != nil {
		return nil, err
	}

	db, err := gorm.Open(mysql.Open(cfg.schemaDSN("")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	return db, nil
}

func runMigrations(cfg dbConfig, dir string) error {
	sqlDB, err := sql.Open("mysql", cfg.schemaDSN("multiStatements=true"))
	if err != nil {
		return fmt.Errorf("open mysql for migrate: %w", err)
	}
	defer sqlDB.Close()

	driver, err := mysqlmigrate.WithInstance(sqlDB, &mysqlmigrate.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	source := "file://" + filepath.ToSlash(dir)
	m, err := migrate.NewWithDatabaseInstance(source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "migrations")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find repo root (go.mod + migrations/) from %s", mustGetwd())
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

func truncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		"SET FOREIGN_KEY_CHECKS = 0",
		"TRUNCATE TABLE comments",
		"TRUNCATE TABLE post_publish_log",
		"TRUNCATE TABLE posts",
		"TRUNCATE TABLE users",
		"SET FOREIGN_KEY_CHECKS = 1",
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error, s)
	}
}
