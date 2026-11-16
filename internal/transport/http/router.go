package http

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/pprof"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/config"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/handler"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/metrics"
	"task-manager-api/pkg/token"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"golang.org/x/time/rate"

	_ "task-manager-api/docs/swagger" // docs is imported for side-effect Swagger registration

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// RouterDependencies encapsulates all dependencies required to construct the HTTP router.
type RouterDependencies struct {
	Config            *config.Config
	Logger            *slog.Logger
	DBPool            *pgxpool.Pool
	Cache             cache.Cache
	Metrics           *metrics.Metrics
	AuthService       service.AuthService
	UserService       service.UserService
	ProjectService    service.ProjectService
	TaskService       service.TaskService
	CommentService    service.CommentService
	AttachmentService service.AttachmentService
	WebhookService    service.WebhookService
	TagService        service.TagService
	TokenMaker        token.Maker
}

// NewRouter constructs and configures the Gin engine with all routes and RFC 9457 error handling.
func NewRouter(deps RouterDependencies) *gin.Engine {
	if deps.Cache == nil {
		deps.Cache = cache.NewNoopCache()
	}

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

	// Configure trusted reverse proxies (e.g. Caddy, Traefik, Docker networks)
	if deps.Config != nil && len(deps.Config.TrustedProxies) > 0 {
		_ = router.SetTrustedProxies(deps.Config.TrustedProxies)
	} else {
		_ = router.SetTrustedProxies(nil)
	}

	maxUploadBytes := int64(10 * 1024 * 1024)
	if deps.Config != nil && deps.Config.MaxUploadSizeMB > 0 {
		maxUploadBytes = deps.Config.MaxUploadSizeMB * 1024 * 1024
	}

	// Attach standard global middlewares
	router.Use(
		otelgin.Middleware("task-manager-api"),
		middleware.RequestID(),
		middleware.StructuredLogger(deps.Logger),
		middleware.Recovery(deps.Logger),
		middleware.SecurityHeaders(deps.Config.AppEnv == "production"),
		middleware.CORS(deps.Config.CORSAllowedOrigins),
		middleware.Timeout(deps.Config.RequestTimeout),
		gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/swagger", "/healthz", "/readyz", "/debug/pprof", "/metrics"})),
		middleware.ETag(),
		middleware.ErrorHandler(deps.Logger),
		middleware.RequestSizeLimiter(2<<20, maxUploadBytes),
	)

	// Attach Prometheus RED metrics middleware if provided
	if deps.Metrics != nil {
		if deps.DBPool != nil {
			deps.Metrics.RegisterDBPoolStats(deps.DBPool)
		}
		router.Use(middleware.Prometheus(deps.Metrics))
		router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(deps.Metrics.Registry, promhttp.HandlerOpts{})))
	}

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

	// Swagger API documentation (enabled only in non-production environments)
	if deps.Config == nil || deps.Config.AppEnv != "production" {
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// Profiling endpoints (net/http/pprof) available in non-production environments
	if deps.Config.AppEnv != "production" {
		pprofGroup := router.Group("/debug/pprof")
		{
			pprofGroup.GET("/", gin.WrapF(pprof.Index))
			pprofGroup.GET("/cmdline", gin.WrapF(pprof.Cmdline))
			pprofGroup.GET("/profile", gin.WrapF(pprof.Profile))
			pprofGroup.POST("/symbol", gin.WrapF(pprof.Symbol))
			pprofGroup.GET("/symbol", gin.WrapF(pprof.Symbol))
			pprofGroup.GET("/trace", gin.WrapF(pprof.Trace))
			pprofGroup.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
			pprofGroup.GET("/block", gin.WrapH(pprof.Handler("block")))
			pprofGroup.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
			pprofGroup.GET("/heap", gin.WrapH(pprof.Handler("heap")))
			pprofGroup.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
			pprofGroup.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
		}
	}

	// API version 1 group
	v1 := router.Group("/api/v1")
	if deps.Config.RateLimitAPILimit > 0 && deps.Config.RateLimitAPIWindow > 0 {
		apiLimiter := middleware.NewRateLimiter(
			deps.Cache,
			"api",
			deps.Config.RateLimitAPILimit,
			deps.Config.RateLimitAPIWindow,
			rate.Limit(deps.Config.RateLimitAPIRate),
			deps.Config.RateLimitAPIBurst,
			deps.Logger,
		)
		v1.Use(apiLimiter.Middleware())
	}
	{
		// Authentication endpoints (public, rate limited per client IP)
		if deps.AuthService != nil {
			authHandler := handler.NewAuthHandler(deps.AuthService)
			authGroup := v1.Group("/auth")
			if deps.Config.RateLimitAuthRate > 0 && deps.Config.RateLimitAuthBurst > 0 {
				authLimit := deps.Config.RateLimitAuthLimit
				authWindow := deps.Config.RateLimitAuthWindow
				if authWindow <= 0 {
					authWindow = time.Minute
				}
				if authLimit <= 0 {
					authLimit = int(math.Ceil(deps.Config.RateLimitAuthRate * authWindow.Seconds()))
					if authLimit <= 0 {
						authLimit = deps.Config.RateLimitAuthBurst
					}
					if authLimit <= 0 {
						authLimit = 10
					}
				}

				authLimiter := middleware.NewRateLimiter(
					deps.Cache,
					"auth",
					authLimit,
					authWindow,
					rate.Limit(deps.Config.RateLimitAuthRate),
					deps.Config.RateLimitAuthBurst,
					deps.Logger,
				)
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
				projectGroup.POST("/:id/members", projectHandler.AddMember)
				projectGroup.GET("/:id/members", projectHandler.ListMembers)
				projectGroup.PATCH("/:id/members/:userId", projectHandler.UpdateMemberRole)
				projectGroup.DELETE("/:id/members/:userId", projectHandler.RemoveMember)
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
				if deps.DBPool != nil {
					taskGroup.POST("", middleware.Idempotency(repository.New(deps.DBPool)), taskHandler.Create)
				} else {
					taskGroup.POST("", taskHandler.Create)
				}
				taskGroup.GET("", taskHandler.List)
				taskGroup.GET("/:id", taskHandler.GetByID)
				taskGroup.PUT("/:id", taskHandler.Update)
				taskGroup.PATCH("/:id", taskHandler.Update)
				taskGroup.DELETE("/:id", taskHandler.Delete)
				taskGroup.PUT("/:id/tags", taskHandler.SetTags)
				if deps.CommentService != nil {
					commentHandler := handler.NewCommentHandler(deps.CommentService)
					taskGroup.POST("/:id/comments", commentHandler.Create)
					taskGroup.GET("/:id/comments", commentHandler.List)
					taskGroup.PATCH("/:id/comments/:commentId", commentHandler.Update)
					taskGroup.DELETE("/:id/comments/:commentId", commentHandler.Delete)
				}
				if deps.AttachmentService != nil {
					attachmentHandler := handler.NewAttachmentHandler(deps.AttachmentService)
					taskGroup.POST("/:id/attachments", attachmentHandler.Upload)
					taskGroup.GET("/:id/attachments", attachmentHandler.List)
					taskGroup.GET("/:id/attachments/:attachmentId", attachmentHandler.Download)
					taskGroup.DELETE("/:id/attachments/:attachmentId", attachmentHandler.Delete)
				}
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

		// Webhook endpoints (protected)
		if deps.WebhookService != nil && deps.TokenMaker != nil {
			webhookHandler := handler.NewWebhookHandler(deps.WebhookService)
			webhookGroup := v1.Group("/webhooks")
			webhookGroup.Use(middleware.AuthMiddleware(deps.TokenMaker))
			{
				webhookGroup.POST("", webhookHandler.Create)
				webhookGroup.GET("", webhookHandler.List)
				webhookGroup.GET("/:id", webhookHandler.GetByID)
				webhookGroup.PATCH("/:id", webhookHandler.Update)
				webhookGroup.DELETE("/:id", webhookHandler.Delete)
				webhookGroup.POST("/:id/test", webhookHandler.TriggerPing)
				webhookGroup.GET("/:id/deliveries", webhookHandler.ListDeliveries)
			}
		}
	}

	return router
}
