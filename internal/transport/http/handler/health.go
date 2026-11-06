package handler

import (
	"context"
	"net/http"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/transport/http/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler manages liveness and readiness probe endpoints.
type HealthHandler struct {
	dbPool *pgxpool.Pool
}

// NewHealthHandler constructs a new HealthHandler.
func NewHealthHandler(dbPool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{dbPool: dbPool}
}

// Liveness indicates if the application process is running.
// GET /healthz
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Readiness verifies that the application dependencies (PostgreSQL) are operational.
// GET /readyz
func (h *HealthHandler) Readiness(c *gin.Context) {
	if h.dbPool == nil {
		reqID := middleware.GetRequestID(c)
		c.Header("Content-Type", "application/problem+json")
		c.JSON(http.StatusServiceUnavailable, domain.NewProblem(
			http.StatusServiceUnavailable,
			"Database pool is uninitialized",
			"DATABASE_UNAVAILABLE",
			c.Request.URL.Path,
			reqID,
		))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.dbPool.Ping(ctx); err != nil {
		reqID := middleware.GetRequestID(c)
		c.Header("Content-Type", "application/problem+json")
		c.JSON(http.StatusServiceUnavailable, domain.NewProblem(
			http.StatusServiceUnavailable,
			"Database ping failed",
			"DATABASE_UNAVAILABLE",
			c.Request.URL.Path,
			reqID,
		))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ready",
		"database": "connected",
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}
