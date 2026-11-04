package handler

import (
	"log/slog"
	"task-manager-api/internal/config"
	"task-manager-api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RouterDependencies encapsulates all dependencies required to construct the HTTP router.
type RouterDependencies struct {
	Config *config.Config
	Logger *slog.Logger
	DBPool *pgxpool.Pool
}

// NewRouter constructs and configures the Gin engine with all routes and middlewares.
func NewRouter(deps RouterDependencies) *gin.Engine {
	if deps.Config.GinMode == gin.ReleaseMode || deps.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else if deps.Config.GinMode == gin.TestMode {
		gin.SetMode(gin.TestMode)
	}

	router := gin.New()

	// Disable trusted proxies by default for security, or set explicitly
	_ = router.SetTrustedProxies(nil)

	// Attach standard middlewares
	router.Use(
		middleware.RequestID(),
		middleware.StructuredLogger(deps.Logger),
		middleware.Recovery(deps.Logger),
	)

	// Health and probe endpoints
	healthHandler := NewHealthHandler(deps.DBPool)
	router.GET("/healthz", healthHandler.Liveness)
	router.GET("/readyz", healthHandler.Readiness)

	// API versioning group
	v1 := router.Group("/api/v1")
	{
		// Domain route groups will be added here in subsequent phases
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})
	}

	return router
}
