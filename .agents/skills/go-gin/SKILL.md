---
name: go-gin
description: >
  Comprehensive guide and best practices for building production-ready REST APIs in Go using the Gin framework.
  Covers clean architecture (Handler-Service-Repository), router setup, middleware, JSON binding & validation,
  error handling, structured responses, connection pooling, and graceful shutdown.
---

# Go & Gin REST API Best Practices

Use this skill whenever developing, refactoring, or reviewing Go REST APIs with the Gin web framework (`github.com/gin-gonic/gin`).

## Architecture & Project Structure

Follow the Standard Layered Architecture (Clean Architecture / Handler-Service-Repository):

```
task-manager-api/
├── cmd/
│   └── api/
│       └── main.go              # Entrypoint, dependency wiring, graceful shutdown
├── internal/
│   ├── config/                  # Environment & app configuration
│   ├── database/                # DB connection pool (pgx / database/sql) & migrations
│   ├── models/                  # Core domain entities & DB representations
│   ├── repository/              # Data access layer (PostgreSQL queries/transactions)
│   ├── service/                 # Business logic & validation rules
│   ├── handler/                 # HTTP controllers (Gin handlers, request binding, status codes)
│   └── middleware/              # Custom Gin middleware (Auth, Logger, CORS, Recovery)
├── pkg/
│   └── response/                # Standardized JSON response helpers
├── migrations/                  # SQL migration files
├── docker-compose.yml           # Local dev services (Postgres, Adminer/pgAdmin)
├── Dockerfile                   # Multi-stage production build
├── go.mod
└── go.sum
```

## Routing & Middleware Rules

1. **Router Initialization**:
   - In production, set `gin.SetMode(gin.ReleaseMode)`.
   - Use `gin.New()` instead of `gin.Default()` when custom logger and recovery formats are needed.
   - Always attach `gin.Recovery()` to avoid crashing on panics.
2. **Route Grouping**:
   - Always group API versions: `v1 := router.Group("/api/v1")`.
   - Keep domain routes grouped: `tasks := v1.Group("/tasks")`.

## Request Handling & Validation

1. **DTOs for Binding**:
   - Separate incoming DTOs (`CreateTaskRequest`, `UpdateTaskRequest`) from database models (`Task`).
   - Use Gin binding tags: `binding:"required,min=3,max=100"`.
   - Use `c.ShouldBindJSON(&req)` rather than `c.BindJSON()` so errors can be inspected and returned gracefully.
2. **Path & Query Parameters**:
   - Validate IDs: use `strconv.Atoi(c.Param("id"))` or UUID validation before querying.
   - For pagination: provide defaults (e.g., `page=1, limit=20`) and upper limits (max limit 100).

## Standardized JSON Response Pattern

All endpoints should return a predictable JSON envelope:

```go
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details string `json:"details,omitempty"`
}
```

- Success 200/201: `response.JSON(c, http.StatusOK, data)`
- Error 400/404/500: `response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())`

## Database & Context Management

1. **Pass `context.Context`**:
   - Always forward `c.Request.Context()` to the service and repository layers:
     `repo.GetByID(c.Request.Context(), id)`
   - Never use `context.Background()` inside HTTP request lifecycles.
2. **Transactions**:
   - Manage multi-step operations in PostgreSQL transactions (`tx, err := db.BeginTx(ctx, nil)`).
   - Ensure `defer tx.Rollback()` is always present before `tx.Commit()`.

## Graceful Shutdown Pattern

Never terminate with `os.Exit(0)` directly. Always listen for OS interrupts:
```go
srv := &http.Server{
    Addr:    ":" + cfg.Port,
    Handler: router,
}

go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("listen: %s\n", err)
    }
}()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := srv.Shutdown(ctx); err != nil {
    log.Fatal("Server forced to shutdown:", err)
}
```
