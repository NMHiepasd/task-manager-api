# ADR-001: Choosing Gin, sqlc, and pgx for Task Manager REST API

- **Status**: Accepted
- **Date**: 2026-11-04
- **Deciders**: Architecture & Engineering Team

---

## 1. Context and Problem Statement

When designing the **Task Manager REST API**, we needed to select our core backend technology stack:
1. An HTTP web framework for routing, request binding, and middleware management.
2. A database access strategy for interacting with PostgreSQL.
3. A PostgreSQL database driver and connection pool.

The chosen stack must balance developer velocity, execution performance, type safety, testability, and operational simplicity.

---

## 2. Decision Drivers

- **Type Safety & Maintainability**: Compile-time verification of queries and schemas to avoid runtime SQL bugs.
- **High Performance & Low Latency**: Minimal memory allocations and reflection overhead.
- **Predictable SQL Control**: Direct visibility into SQL execution plans, indexing strategies, and query performance without black-box ORM query generation.
- **Robust Ecosystem & Tooling**: Mature community, rich middleware support, and production-proven reliability.

---

## 3. Considered Options

### Web Framework:
- **Option 1**: Gin (`github.com/gin-gonic/gin`) - *Selected*
- **Option 2**: Chi (`github.com/go-chi/chi`)
- **Option 3**: Fiber (`github.com/gofiber/fiber`)
- **Option 4**: Standard library `net/http`

### Data Access Layer:
- **Option 1**: `sqlc` (Type-safe SQL compiler) - *Selected*
- **Option 2**: GORM (`gorm.io/gorm`)
- **Option 3**: Raw `database/sql` with hand-written scanning
- **Option 4**: Ent / SQLBoiler

### Database Driver:
- **Option 1**: `jackc/pgx/v5` - *Selected*
- **Option 2**: `lib/pq` (Legacy)
- **Option 3**: Standard `database/sql` over generic driver

---

## 4. Decision & Rationale

We decided to adopt **Gin + sqlc + pgx/v5**.

### 4.1. Why Gin?
- **Speed & Efficiency**: Gin utilizes a custom Radix-tree based router, yielding low memory footprint and high request throughput.
- **Built-in Validation & Binding**: Built-in integration with `go-playground/validator` allows declarative request validation (`binding:"required,min=3"`).
- **Mature Middleware**: Standardized error recovery, logging, CORS, and authentication middleware.
- **Predictable API**: Idiomatic context management and clean Handler-Service separation.

### 4.2. Why sqlc?
- **Compile-time Verification**: `sqlc` validates SQL queries directly against our PostgreSQL schema during compilation. Syntax errors and column mismatches are detected before runtime.
- **Zero Reflection Overhead**: Unlike heavy ORMs (such as GORM), `sqlc` generates pure, readable Go structs and methods with direct row scanning.
- **SQL-First Mindset**: Full control over query writing, joins, PostgreSQL-specific features (e.g., JSONB, `RETURNING`, indexing), eliminating the "N+1 query" problems typical of ORMs.
- **Seamless Testability**: Generated interfaces can be mocked or verified against ephemeral test databases effortlessly.

### 4.3. Why pgx/v5?
- **Modern PostgreSQL Driver**: `lib/pq` is effectively in maintenance mode. `pgx` is the standard for modern Go + PostgreSQL applications.
- **Optimized Connection Pooling**: Native connection pool management (`puddle` / `pgxpool`) providing granular configuration for idle connections, max lifetimes, and health checks.
- **PostgreSQL Native Features**: Full support for PostgreSQL binary format, COPY protocols, prepared statements, and complex types (UUID, Timestamps with Timezone).

---

## 5. Consequences and Trade-offs

### Positive Consequences:
- **Strict Compile-time Guarantees**: Any database schema modification forces an update to queries and generated Go code, preventing silent breakages.
- **High Performance**: Minimal allocation path from HTTP request to database query response.
- **Developer Clarity**: Junior and senior engineers alike can read plain SQL and standard Go handlers without deciphering ORM abstraction layers.

### Trade-offs:
- Requires running `sqlc generate` whenever SQL schema or queries are modified (automated via `Makefile`).
- Complex dynamic queries with dozens of optional filters require either query building patterns or carefully designed conditional SQL blocks.

---

## 6. References
- [Gin Web Framework Documentation](https://gin-gonic.com/)
- [sqlc: Compile SQL to type-safe Go](https://sqlc.dev/)
- [pgx: PostgreSQL driver and toolkit for Go](https://github.com/jackc/pgx)
