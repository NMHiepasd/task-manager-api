.PHONY: help run build test lint migrate-up migrate-down sqlc docker-up docker-down

# Default target
.DEFAULT_GOAL := help

# Database configuration variables
DB_URL ?= postgres://postgres:postgres@localhost:5432/task_manager?sslmode=disable

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@echo "  run           Run the application locally"
	@echo "  build         Build the binary executable into bin/"
	@echo "  test          Run unit and integration tests"
	@echo "  lint          Run golangci-lint on the codebase"
	@echo "  migrate-up    Apply all pending database migrations"
	@echo "  migrate-down  Rollback the most recent database migration"
	@echo "  sqlc          Generate type-safe Go database code from SQL queries"
	@echo "  docker-up     Start Docker containers (PostgreSQL, Adminer)"
	@echo "  docker-down   Stop and remove Docker containers"

run: ## Run the API server locally
	go run ./cmd/api

build: ## Build API binary
	go build -ldflags="-w -s" -o bin/api ./cmd/api

test: ## Run unit and integration tests
	go test -v -race -coverprofile=coverage.out ./...

lint: ## Run golangci-lint
	MSYS_NO_PATHCONV=1 docker run --rm -v "${PWD}:/app" -w //app golangci/golangci-lint:latest golangci-lint run ./...

migrate-up: ## Run database migrations up
	docker exec -i task_manager_db psql -U postgres -d task_manager -c "$$(cat migrations/000001_create_initial_schema.up.sql)"

migrate-down: ## Rollback migrations
	docker exec -i task_manager_db psql -U postgres -d task_manager -c "$$(cat migrations/000001_create_initial_schema.down.sql)"

sqlc: ## Generate type-safe Go code with sqlc
	MSYS_NO_PATHCONV=1 docker run --rm -v "${PWD}:/src" -w //src sqlc/sqlc:latest generate

docker-up: ## Start Docker services
	docker compose up -d

docker-down: ## Stop Docker services
	docker compose down
