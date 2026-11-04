package handler

import (
	"context"
	"net/http"
	"task-manager-api/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler manages liveness and readiness probe endpoints.
type HealthHandler struct {
	pool *pgxpool.Pool
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// Liveness handles GET /healthz (Kubernetes/Docker liveness probe).
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, response.Success(gin.H{
		"status": "alive",
		"time":   time.Now().UTC(),
	}))
}

// Readiness handles GET /readyz (Kubernetes/Docker readiness probe).
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if h.pool == nil {
		c.JSON(http.StatusServiceUnavailable, response.Error(
			"DATABASE_UNAVAILABLE",
			"Database connection pool is not initialized",
		))
		return
	}

	if err := h.pool.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, response.Error(
			"DATABASE_DISCONNECTED",
			"Database connection ping failed",
			err.Error(),
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"status":   "ready",
		"database": "connected",
		"time":     time.Now().UTC(),
	}))
}
