# Task Manager REST API

[![Go Report Card](https://goreportcard.com/badge/github.com/NMHiepasd/task-manager-api)](https://goreportcard.com/report/github.com/NMHiepasd/task-manager-api)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Production-ready Task Manager REST API in Go using Gin, PostgreSQL, sqlc, and pgx with clean layered architecture, containerization, and automated tooling.

---

## Tech Stack & Architecture

- **Language**: Go 1.24+
- **HTTP Framework**: [Gin](https://github.com/gin-gonic/gin) (Radix tree router, validation, middlewares)
- **Database**: PostgreSQL 16
- **Database Driver & Pool**: [pgx/v5](https://github.com/jackc/pgx)
- **Type-Safe SQL**: [sqlc](https://sqlc.dev/) (Compiles raw SQL into type-safe Go structs and queries)
- **Containerization**: Docker & Docker Compose
- **Live Reload**: [Air](https://github.com/air-verse/air)
- **Linting & Code Quality**: [golangci-lint](https://golangci-lint.run/)
- **Git Hooks**: [Lefthook](https://github.com/evilmartians/lefthook)

### Clean Layered Architecture

```
task-manager-api/
├── cmd/
│   └── api/
│       └── main.go              # Entrypoint, dependency wiring, graceful shutdown
├── internal/
│   ├── config/                  # App environment configuration
│   ├── database/                # Database connection pooling and health checks
│   ├── models/                  # Domain entities and DTOs
│   ├── repository/              # Data access layer (sqlc generated & DB queries)
│   ├── service/                 # Business logic and rules
│   ├── handler/                 # HTTP controllers (Gin handlers, binding, status)
│   └── middleware/              # Custom middlewares (Auth, Logger, CORS, Recovery)
├── pkg/
│   └── response/                # Standardized JSON response envelope
├── queries/                     # Raw SQL queries compiled by sqlc
├── migrations/                  # SQL migration files
├── docs/
│   └── adr/                     # Architecture Decision Records (e.g. ADR-001)
├── docker-compose.yml           # Local dev services (PostgreSQL, Adminer)
├── Dockerfile                   # Multi-stage production build
├── .air.toml                    # Air hot-reload configuration
├── .golangci.yml                # Strict linter configuration
├── lefthook.yml                 # Pre-commit & commit-msg Git hooks
├── Makefile                     # Common development commands
├── go.mod
└── go.sum
```

---

## Getting Started

### Prerequisites
- [Go](https://golang.org/dl/) (version 1.24+)
- [Docker & Docker Compose](https://www.docker.com/)
- Optional: `make`, `golangci-lint`, `air`, `sqlc`, `lefthook`

### 1. Clone & Setup Environment
```bash
git clone https://github.com/NMHiepasd/task-manager-api.git
cd task-manager-api
cp .env.example .env
```

### 2. Start PostgreSQL & Adminer
```bash
# Start database and Adminer via Docker Compose
make docker-up
# or: docker compose up -d
```
- **PostgreSQL**: `localhost:5432` (User: `postgres`, Password: `postgres`, DB: `task_manager`)
- **Adminer UI**: [http://localhost:8081](http://localhost:8081) (System: `PostgreSQL`, Server: `db`)

### 3. Run the API Server
```bash
# Run server
make run
# or with hot-reload (if Air is installed):
air
```

Verify the health check endpoint:
```bash
curl http://localhost:8080/health
```
Response:
```json
{
  "success": true,
  "data": {
    "database": "connected",
    "status": "ok",
    "time": "2026-11-04T10:00:00Z"
  }
}
```

---

## Available Commands (`Makefile`)

| Command | Description |
| :--- | :--- |
| `make run` | Run the application locally |
| `make build` | Compile the binary into `bin/` |
| `make test` | Run unit and integration tests with race detection and coverage |
| `make lint` | Run `golangci-lint` with strict linters enabled |
| `make migrate-up` | Apply pending database schema migrations |
| `make migrate-down` | Rollback database migrations |
| `make sqlc` | Generate type-safe Go database code from SQL queries |
| `make docker-up` | Start Docker containers (PostgreSQL, Adminer) in detached mode |
| `make docker-down` | Stop and remove running Docker containers |

---

## Git Workflow & Conventional Commits

All commit messages in this repository must follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <short description>
```

### Common Types:
- `feat`: A new feature or endpoint (e.g., `feat(task): implement create task endpoint`)
- `fix`: A bug fix (e.g., `fix(db): correct connection pool timeout`)
- `docs`: Documentation changes only (e.g., `docs(adr): add ADR-001 for Gin and sqlc`)
- `refactor`: A code change that neither fixes a bug nor adds a feature
- `test`: Adding missing tests or correcting existing tests
- `chore`: Build process, dependencies, or auxiliary tool changes (e.g., `chore(lint): update golangci config`)

---

## Architecture Decision Records (ADR)
Key architectural decisions are documented under [`docs/adr/`](docs/adr/):
- [ADR-001: Choosing Gin, sqlc, and pgx](docs/adr/0001-why-gin-sqlc-pgx.md)

---

## License
This project is open-source and available under the [MIT License](LICENSE).
