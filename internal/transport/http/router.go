package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"task-manager-api/internal/config"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/handler"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/token"

	"golang.org/x/time/rate"

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

// NewRouter constructs and configures the Gin engine with all routes and RFC 9457 error handling.
func NewRouter(deps RouterDependencies) *gin.Engine {
	if deps.Config.GinMode == gin.ReleaseMode || deps.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else if deps.Config.GinMode == gin.TestMode {
		gin.SetMode(gin.TestMode)
	}

	router := gin.New()

	// Disable auto redirects per ADR-002 for strict REST semantics
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.HandleMethodNotAllowed = true

	// Disable trusted proxies by default for security
	_ = router.SetTrustedProxies(nil)

	// Attach standard global middlewares
	router.Use(
		middleware.RequestID(),
		middleware.StructuredLogger(deps.Logger),
		middleware.Recovery(deps.Logger),
		middleware.SecurityHeaders(deps.Config.AppEnv == "production"),
		middleware.CORS(deps.Config.CORSAllowedOrigins),
		middleware.MaxBytes(2<<20), // 2MB request body ceiling
		middleware.ErrorHandler(deps.Logger),
	)

	// NoRoute handler: RFC 9457 404
	router.NoRoute(func(c *gin.Context) {
		reqID := middleware.GetRequestID(c)
		c.Header("Content-Type", "application/problem+json")
		c.JSON(http.StatusNotFound, domain.NewProblem(
			http.StatusNotFound,
			fmt.Sprintf("Route %s %s was not found on this server", c.Request.Method, c.Request.URL.Path),
			"ROUTE_NOT_FOUND",
			c.Request.URL.Path,
			reqID,
		))
	})

	// NoMethod handler: RFC 9457 405
	router.NoMethod(func(c *gin.Context) {
		reqID := middleware.GetRequestID(c)
		c.Header("Content-Type", "application/problem+json")
		c.JSON(http.StatusMethodNotAllowed, domain.NewProblem(
			http.StatusMethodNotAllowed,
			fmt.Sprintf("Method %s is not allowed for route %s", c.Request.Method, c.Request.URL.Path),
			"METHOD_NOT_ALLOWED",
			c.Request.URL.Path,
			reqID,
		))
	})

	// Health and probe endpoints (unauthenticated)
	healthHandler := handler.NewHealthHandler(deps.DBPool)
	router.GET("/healthz", healthHandler.Liveness)
	router.GET("/readyz", healthHandler.Readiness)

	// API version 1 group
	v1 := router.Group("/api/v1")
	{
		// Authentication endpoints (public, rate limited per client IP)
		if deps.AuthService != nil {
			authHandler := handler.NewAuthHandler(deps.AuthService)
			authGroup := v1.Group("/auth")
			if deps.Config.RateLimitAuthRate > 0 && deps.Config.RateLimitAuthBurst > 0 {
				authLimiter := middleware.NewIPRateLimiter(rate.Limit(deps.Config.RateLimitAuthRate), deps.Config.RateLimitAuthBurst)
				authGroup.Use(authLimiter.Middleware())
			}
			{
				authGroup.POST("/register", authHandler.Register)
				authGroup.POST("/login", authHandler.Login)
				authGroup.POST("/refresh", authHandler.RefreshToken)
				authGroup.POST("/logout", authHandler.Logout)
			}
		}

		// User profile endpoints (protected)
		if deps.UserService != nil && deps.TokenMaker != nil {
			userHandler := handler.NewUserHandler(deps.UserService)
			userGroup := v1.Group("/users")
			userGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				userGroup.GET("/me", userHandler.GetMe)
			}
		}

		// Project endpoints (protected)
		if deps.ProjectService != nil && deps.TokenMaker != nil {
			projectHandler := handler.NewProjectHandler(deps.ProjectService)
			projectGroup := v1.Group("/projects")
			projectGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				projectGroup.POST("", projectHandler.Create)
				projectGroup.GET("", projectHandler.List)
				projectGroup.GET("/:id", projectHandler.GetByID)
				projectGroup.PUT("/:id", projectHandler.Update)
				projectGroup.PATCH("/:id", projectHandler.Update)
				projectGroup.DELETE("/:id", projectHandler.Delete)
				if deps.TaskService != nil {
					taskHandler := handler.NewTaskHandler(deps.TaskService)
					projectGroup.GET("/:id/tasks", taskHandler.ListProjectTasks)
				}
			}
		}

		// Task endpoints (protected)
		if deps.TaskService != nil && deps.TokenMaker != nil {
			taskHandler := handler.NewTaskHandler(deps.TaskService)
			taskGroup := v1.Group("/tasks")
			taskGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				taskGroup.POST("", taskHandler.Create)
				taskGroup.GET("", taskHandler.List)
				taskGroup.GET("/:id", taskHandler.GetByID)
				taskGroup.PUT("/:id", taskHandler.Update)
				taskGroup.PATCH("/:id", taskHandler.Update)
				taskGroup.DELETE("/:id", taskHandler.Delete)
				taskGroup.PUT("/:id/tags", taskHandler.SetTags)
			}
		}

		// Tag endpoints (protected)
		if deps.TagService != nil && deps.TokenMaker != nil {
			tagHandler := handler.NewTagHandler(deps.TagService)
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
