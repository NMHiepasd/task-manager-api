package handler

import (
	"log/slog"
	"task-manager-api/internal/config"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RouterDependencies encapsulates all dependencies required to construct the HTTP router.
type RouterDependencies struct {
	Config         *config.Config
	Logger         *slog.Logger
	DBPool         *pgxpool.Pool
	AuthService    service.AuthService
	UserService    service.UserService
	ProjectService service.ProjectService
	TaskService    service.TaskService
	TagService     service.TagService
	TokenMaker     token.Maker
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
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})

		// Authentication endpoints (public)
		if deps.AuthService != nil {
			authHandler := NewAuthHandler(deps.AuthService)
			authGroup := v1.Group("/auth")
			{
				authGroup.POST("/register", authHandler.Register)
				authGroup.POST("/login", authHandler.Login)
				authGroup.POST("/refresh", authHandler.RefreshToken)
				authGroup.POST("/logout", authHandler.Logout)
			}
		}

		// User endpoints (protected by JWT AuthMiddleware)
		if deps.UserService != nil && deps.TokenMaker != nil {
			userHandler := NewUserHandler(deps.UserService)
			userGroup := v1.Group("/users")
			userGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				userGroup.GET("/me", userHandler.GetMe)
			}
		}

		// Project endpoints (protected by JWT AuthMiddleware)
		if deps.ProjectService != nil && deps.TokenMaker != nil {
			projectHandler := NewProjectHandler(deps.ProjectService)
			projectGroup := v1.Group("/projects")
			projectGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				projectGroup.POST("", projectHandler.Create)
				projectGroup.GET("", projectHandler.List)
				projectGroup.GET("/:id", projectHandler.GetByID)
				projectGroup.PUT("/:id", projectHandler.Update)
				projectGroup.DELETE("/:id", projectHandler.Delete)
			}
		}

		// Task endpoints (protected by JWT AuthMiddleware)
		if deps.TaskService != nil && deps.TokenMaker != nil {
			taskHandler := NewTaskHandler(deps.TaskService)
			taskGroup := v1.Group("/tasks")
			taskGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				taskGroup.POST("", taskHandler.Create)
				taskGroup.GET("", taskHandler.List)
				taskGroup.GET("/:id", taskHandler.GetByID)
				taskGroup.PUT("/:id", taskHandler.Update)
				taskGroup.DELETE("/:id", taskHandler.Delete)
			}
		}

		// Tag endpoints (protected by JWT AuthMiddleware)
		if deps.TagService != nil && deps.TokenMaker != nil {
			tagHandler := NewTagHandler(deps.TagService)
			tagGroup := v1.Group("/tags")
			tagGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				tagGroup.POST("", tagHandler.Create)
				tagGroup.GET("", tagHandler.List)
				tagGroup.DELETE("/:id", tagHandler.Delete)
			}
		}
	}

	return router
}
