.PHONY: run build test test-coverage test-integration migrate-install migrate-up migrate-down docker-up docker-down

APP_NAME := blog-api
MIGRATE  ?= migrate
MIGRATIONS_DIR := migrations

# Load .env if present (Make does not export by default for recipes).
ifneq (,$(wildcard .env))
    include .env
    export
endif

DATABASE_URL ?= mysql://$(DB_USER):$(DB_PASSWORD)@tcp($(DB_HOST):$(DB_PORT))/$(DB_NAME)?multiStatements=true

# Install CLI with MySQL driver support (pin matches go.mod):
#   go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3
migrate-install:
	go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

run:
	go run ./cmd/api

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

# HTTP handler tests against a real MySQL schema (see TESTING.md).
# -p 1: feature packages share TEST_DB_NAME and must not truncate in parallel.
test-integration:
	go test -tags=integration -p 1 -count=1 ./internal/user ./internal/post ./internal/comment

migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
