# Task Manager REST API

[![Go Report Card](https://goreportcard.com/badge/github.com/NMHiepasd/task-manager-api)](https://goreportcard.com/report/github.com/NMHiepasd/task-manager-api)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![OpenAPI Spec](https://img.shields.io/badge/OpenAPI-3.0%20%2F%20Swagger-green.svg)](http://localhost:8080/swagger/index.html)

Production-ready, highly secure Task Manager REST API built in Go adhering to **Clean Architecture** principles, strict **RFC 9457 Problem Details for HTTP APIs**, **OWASP Security Hardening**, **Keyset Cursor-based Pagination**, **Full-Text Search (GIN)**, and comprehensive multi-tier testing (**Testcontainers**, **Fuzzing**, **Race detection**).

---

## Table of Contents

- [Features](#features)
- [Architecture & Tech Stack](#architecture--tech-stack)
  - [Clean Architecture Flow](#clean-architecture-flow)
  - [Database Entity-Relationship Diagram (ERD)](#database-entity-relationship-diagram-erd)
  - [Project Directory Layout](#project-directory-layout)
- [API Documentation & Swagger UI](#api-documentation--swagger-ui)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Environment Configuration](#environment-configuration)
  - [Quickstart with Docker](#quickstart-with-docker)
  - [Local Development Setup](#local-development-setup)
- [Testing Strategy](#testing-strategy)
- [Available Commands (`Makefile`)](#available-commands-makefile)
- [Architecture Decision Records (ADR)](#architecture-decision-records-adr)
- [License](#license)

---

## Features

- **Clean Architecture & Separation of Concerns**: Handler $\to$ Service $\to$ Repository $\to$ Database.
- **Interactive OpenAPI 3.0 / Swagger UI**: Fully typed and accessible at `/swagger/index.html`.
- **RFC 9457 Problem Details**: Standardized structured JSON error payloads (`application/problem+json`).
- **OWASP Security Hardening**: Strict Content Security Policy (CSP), HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, and CORS allowlisting.
- **In-Memory Token Bucket Rate Limiting**: Per-client IP rate limiter for public authentication endpoints.
- **Anti-IDOR & Information Leak Protection**: Returns `404 Not Found` (instead of `403 Forbidden`) when accessing or mutating resources not owned by the tenant.
- **Database Connection Pooling & Transactions**: High-performance connection pooling via `jackc/pgx/v5` with explicit transactional scope helper `WithTx`.
- **Keyset Cursor Pagination & Full-Text Search**: Scalable Base64 cursor pagination (`(created_at, id)`) and PostgreSQL `tsvector`/GIN full-text search.
- **Multi-tier Automated Testing**:
  - Unit tests with `go test -short`.
  - Integration tests using real PostgreSQL instances with **Testcontainers-Go**.
  - Fuzz tests for binary-level cursor decoding robustness.
  - Race condition detection (`-race`) and goroutine leak detection (`go.uber.org/goleak`).

---

## Architecture & Tech Stack

- **Language**: Go 1.24+
- **HTTP Engine**: [Gin](https://github.com/gin-gonic/gin) (Radix tree router, validation, middlewares)
- **Database**: PostgreSQL 16+
- **Database Driver & Pool**: [pgx/v5](https://github.com/jackc/pgx)
- **Type-Safe SQL Compilation**: [sqlc](https://sqlc.dev/)
- **Documentation**: [swaggo/swag](https://github.com/swaggo/swag) & [gin-swagger](https://github.com/swaggo/gin-swagger)
- **Testing Containers**: [testcontainers-go](https://golang.testcontainers.org/)
- **Linting & Code Quality**: [golangci-lint](https://golangci-lint.run/)

### Clean Architecture Flow

```mermaid
flowchart LR
    Client([HTTP Client]) -->|Request| MW[Middlewares<br/>Logger, CORS, RateLimit, Auth]
    MW -->|c.Context| H[Handler Layer<br/>HTTP Controllers & DTOs]
    H -->|Input DTOs| S[Service Layer<br/>Business Logic & Transactions]
    S -->|Domain Models| R[Repository Layer<br/>sqlc & pgxpool.Pool]
    R -->|SQL Queries| DB[(PostgreSQL 16)]
    DB -.->|Result Sets| R
    R -.->|Entities| S
    S -.->|Response DTOs| H
    H -.->|JSON / RFC 9457| Client
```

### Database Entity-Relationship Diagram (ERD)

```mermaid
erDiagram
    USERS ||--o{ PROJECTS : "owns"
    USERS ||--o{ TASKS : "owns"
    USERS ||--o{ TAGS : "creates"
    PROJECTS ||--o{ TASKS : "contains"
    TASKS ||--o{ TASK_TAGS : "tagged with"
    TAGS ||--o{ TASK_TAGS : "applied to"

    USERS {
        uuid id PK
        varchar email UK
        varchar password_hash
        varchar full_name
        varchar role
        timestamptz created_at
        timestamptz updated_at
    }

    PROJECTS {
        uuid id PK
        uuid user_id FK
        varchar name
        text description
        varchar status
        timestamptz created_at
        timestamptz updated_at
    }

    TASKS {
        uuid id PK
        uuid user_id FK
        uuid project_id FK
        varchar title
        text description
        varchar status
        varchar priority
        date due_date
        tsvector search_vector
        timestamptz created_at
        timestamptz updated_at
    }

    TAGS {
        uuid id PK
        uuid user_id FK
        varchar name
        varchar color
        timestamptz created_at
    }

    TASK_TAGS {
        uuid task_id PK,FK
        uuid tag_id PK,FK
    }
```

### Project Directory Layout

```
task-manager-api/
├── cmd/
│   └── api/
│       └── main.go              # Application entrypoint, dependency wiring, graceful shutdown
├── internal/
│   ├── config/                  # Environment variable configuration (caarlos0/env)
│   ├── database/                # pgx connection pooling and database health check
│   ├── domain/                  # Enterprise domain entities, interfaces, and RFC 9457 Problem Details
│   ├── repository/              # Data access implementation (sqlc compiled code & integration tests)
│   ├── service/                 # Core domain business logic and transaction management
│   └── transport/
│       └── http/
│           ├── handler/         # HTTP controller handlers & request/response DTOs
│           ├── middleware/      # Auth, CORS, Logger, RequestID, RateLimiter, Recovery, Security
│           └── router.go        # Router configuration and endpoint routing table
├── pkg/
│   ├── hasher/                  # Password hashing (Argon2id / bcrypt)
│   ├── logger/                  # Structured logging (log/slog)
│   ├── pagination/              # Keyset cursor pagination encoder/decoder
│   ├── response/                # Standardized envelope response helpers
│   └── token/                   # JWT & PASETO token generator and validator
├── docs/
│   ├── adr/                     # Architecture Decision Records
│   └── swagger/                 # OpenAPI 3.0 generated specifications (docs.go, swagger.json)
├── queries/                     # SQL queries compiled by sqlc
├── migrations/                  # Schema migration files
├── bruno/                       # Bruno API Client collection
├── docker-compose.yml           # Local dev orchestrator (PostgreSQL, Adminer, API)
├── Dockerfile                   # Multi-stage container build
├── Makefile                     # Build, test, migration, and tooling targets
├── go.mod
└── go.sum
```

---

## API Documentation & Swagger UI

Once the application is running, you can access the interactive Swagger UI at:

👉 **[http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)**

### Key Endpoint Groups:
- **Auth**: `/api/v1/auth/register`, `/api/v1/auth/login`, `/api/v1/auth/refresh`, `/api/v1/auth/logout`
- **Users**: `/api/v1/users/me`
- **Projects**: `/api/v1/projects` (CRUD + `/tasks` nested sub-resource)
- **Tasks**: `/api/v1/tasks` (CRUD, Keyset cursor pagination, Full-Text Search, `/tags` assignment)
- **Tags**: `/api/v1/tags` (CRUD)
- **Health Probes**: `/healthz` (liveness), `/readyz` (readiness)

---

## Getting Started

### Prerequisites
- [Go](https://golang.org/dl/) (version 1.24+)
- [Docker & Docker Compose](https://www.docker.com/)
- Optional: `make`, `swag`, `golangci-lint`

### Environment Configuration

Create a local `.env` file from the example:
```bash
cp .env.example .env
```

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `APP_ENV` | `development` | Environment (`development`, `staging`, `production`) |
| `HTTP_PORT` | `8080` | Server listening port |
| `GIN_MODE` | `debug` | Gin operating mode (`debug`, `release`, `test`) |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/task_manager?sslmode=disable` | PostgreSQL connection string |
| `DB_MAX_CONNS` | `25` | Maximum active DB connections |
| `DB_MIN_CONNS` | `5` | Minimum idle DB connections |
| `DB_MAX_CONN_LIFETIME` | `1h` | Maximum connection reuse duration |
| `DB_MAX_CONN_IDLE_TIME` | `30m` | Maximum connection idle duration |
| `JWT_SECRET` | `task-manager-dev-secret-key-32bytes-min!` | Secret key for signing access tokens |
| `JWT_ACCESS_DURATION` | `15m` | Lifetime of access tokens |
| `JWT_REFRESH_DURATION`| `168h` (7 days) | Lifetime of refresh tokens |
| `RATE_LIMIT_AUTH_RATE`| `5` | Allowed requests per second per IP for auth |
| `RATE_LIMIT_AUTH_BURST`| `10` | Allowed burst limit for auth |
| `CORS_ALLOWED_ORIGINS`| `http://localhost:3000,http://localhost:5173` | Comma-separated allowed origins |

### Quickstart with Docker

To boot the entire stack (PostgreSQL, Adminer, and the Task Manager API) in isolated containers:

```bash
docker compose up --build -d
```
- **API Server & Swagger**: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- **Adminer Database UI**: [http://localhost:8081](http://localhost:8081)

### Local Development Setup

1. Start database and UI tooling:
   ```bash
   make docker-up
   ```

2. Apply database migrations:
   ```bash
   make migrate-up
   ```

3. Run the API locally:
   ```bash
   make run
   ```

4. Regenerate Swagger documentation after handler changes:
   ```bash
   make swagger
   ```

---

## Testing Strategy

The repository employs a multi-tiered automated testing regimen:

```bash
# 1. Run unit tests
make test

# 2. Run unit tests with the race detector
make test-race

# 3. Run integration tests with Testcontainers (requires Docker)
make test-integration

# 4. Run fuzz testing on cursor pagination
make test-fuzz

# 5. Generate coverage report
make cover
```

---

## Available Commands (`Makefile`)

| Target | Description |
| :--- | :--- |
| `make run` | Run API server locally with Go |
| `make build` | Compile optimized production binary into `bin/` |
| `make test` | Run fast unit tests (`-short`) |
| `make test-race` | Run unit tests with `-race` detection |
| `make test-integration` | Execute integration tests against real PostgreSQL via Testcontainers |
| `make test-fuzz` | Run property fuzz tests against cursor serialization/deserialization |
| `make bench` | Run microbenchmarks on hot paths (pagination, JWT, Redis cache) |
| `make load-test` | Execute k6 concurrent load testing script |
| `make cover` | Produce HTML code coverage profile |
| `make lint` | Run `golangci-lint` with strict security and style rules |
| `make swagger` | Generate Swagger OpenAPI specifications via `swaggo/swag` |
| `make migrate-up` | Execute SQL schema migrations up |
| `make migrate-down` | Rollback schema migrations |
| `make sqlc` | Compile type-safe Go repository code from `queries/` |
| `make docker-up` | Start background database and tooling containers |
| `make docker-down` | Stop running containers |

---

## Architecture Decision Records (ADR)

Detailed engineering decision rationale is cataloged in [`docs/adr/`](docs/adr/):
- **[ADR-001: Choosing Gin, sqlc, and pgx](docs/adr/0001-why-gin-sqlc-pgx.md)**
- **[ADR-002: Clean Architecture and RFC 9457 Problem Details](docs/adr/0002-clean-architecture-and-rfc9457.md)**
- **[ADR-003: Security Hardening, Rate Limiting, and CORS](docs/adr/0003-security-hardening-ratelimit-cors.md)**
- **[ADR-004: Keyset Pagination, Full-Text Search, and Anti-IDOR Strategy](docs/adr/0004-keyset-pagination-fts-anti-idor.md)**
- **[ADR-005: Testing Strategy with Testcontainers, Fuzzing, and Goleak](docs/adr/0005-testing-strategy-and-testcontainers.md)**
- **[ADR-006: API Documentation with OpenAPI 3.0 and Swagger UI](docs/adr/0006-api-documentation-and-swagger.md)**
- **[ADR-007: Idempotency-Key Header and Request Timeout Strategy](docs/adr/0007-idempotency-key-and-request-timeout.md)**
- **[ADR-008: Caching Strategy and Performance Optimization](docs/adr/0008-caching-strategy-and-performance-optimization.md)**
- Detailed performance analysis and benchmark reports are documented in **[`docs/performance.md`](docs/performance.md)**.

---

## License

This project is licensed under the [MIT License](LICENSE).
