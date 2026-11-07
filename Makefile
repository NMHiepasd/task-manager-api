.PHONY: help run build test test-race test-integration test-fuzz cover lint migrate-up migrate-down sqlc docker-up docker-down

# Default target
.DEFAULT_GOAL := help

# Database configuration variables
DB_URL ?= postgres://postgres:postgres@localhost:5432/task_manager?sslmode=disable

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@echo "  run               Run the application locally"
	@echo "  build             Build the binary executable into bin/"
	@echo "  test              Run fast unit tests"
	@echo "  test-race         Run unit tests with race detection"
	@echo "  test-integration  Run repository integration tests with testcontainers"
	@echo "  test-fuzz         Run fuzz tests on cursor pagination"
	@echo "  cover             Generate test coverage profile and HTML report"
	@echo "  lint              Run golangci-lint on the codebase"
	@echo "  migrate-up        Apply all pending database migrations"
	@echo "  migrate-down      Rollback the most recent database migration"
	@echo "  sqlc              Generate type-safe Go database code from SQL queries"
	@echo "  docker-up         Start Docker containers (PostgreSQL, Adminer)"
	@echo "  docker-down       Stop and remove Docker containers"

run: ## Run the API server locally
	go run ./cmd/api

build: ## Build API binary
	go build -ldflags="-w -s" -o bin/api ./cmd/api

test: ## Run unit tests
	go test -v -short ./...

test-race: ## Run unit tests with race detector
	go test -v -race -short ./...

test-integration: ## Run integration tests with Testcontainers
	go test -v -tags=integration -timeout 180s ./internal/repository/...

test-fuzz: ## Run fuzz tests
	go test -fuzz=FuzzDecodeCursor -fuzztime=5s ./pkg/pagination/...

cover: ## Generate test coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

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
