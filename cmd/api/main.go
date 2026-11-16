// @title           Task Manager REST API
// @version         1.0.0
// @description     Production-ready RESTful Task Management API built with Go, Gin, PostgreSQL (pgx/v5 & sqlc), and Docker.
// @contact.name    API Support
// @contact.url     https://github.com/NMHiepasd/task-manager-api
// @license.name    MIT
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Type "Bearer" followed by a space and your JWT access token.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/config"
	"task-manager-api/internal/database"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/service"
	"task-manager-api/internal/worker"
	"task-manager-api/pkg/hasher"
	"task-manager-api/pkg/logger"
	"task-manager-api/pkg/metrics"
	"task-manager-api/pkg/storage"
	"task-manager-api/pkg/token"
	"task-manager-api/pkg/tracer"
	"time"

	"github.com/hibiken/asynq"
	transportHttp "task-manager-api/internal/transport/http"
)

// version is injected at build time via -ldflags="-X main.version=..."
var version = "dev"

func main() {
	// 1. Load and validate environment configuration (fail-fast)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	// 2. Initialize structured JSON logger
	log := logger.Setup(cfg.LogLevel, cfg.AppEnv)
	log.Info("Configuration loaded successfully",
		"version", version,
		"env", cfg.AppEnv,
		"port", cfg.AppPort,
		"log_level", cfg.LogLevel,
	)

	// 3. Connect to PostgreSQL using connection pool
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("PostgreSQL connection pool established",
		"max_conns", cfg.DBMaxConns,
		"min_conns", cfg.DBMinConns,
	)

	// 4. Initialize Redis cache (with graceful fallback to NoopCache if unavailable)
	var cacheStore cache.Cache
	redisCache, err := cache.NewRedisCache(ctx, cfg.RedisURL, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Warn("Redis unavailable; operating with in-memory noop cache", "error", err)
		cacheStore = cache.NewNoopCache()
	} else {
		defer redisCache.Close()
		cacheStore = redisCache
		log.Info("Redis cache connection established", "url", maskRedisURL(cfg.RedisURL))
	}

	// 5. Initialize OpenTelemetry tracing
	_, tracerCleanup, err := tracer.InitTracer(ctx, "task-manager-api", cfg.AppEnv)
	if err != nil {
		log.Warn("Failed to initialize OpenTelemetry tracer", "error", err)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = tracerCleanup(shutdownCtx)
		}()
	}

	// 6. Initialize Prometheus RED metrics
	appMetrics := metrics.NewMetrics()

	// 7. Initialize repository, services, and token maker
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		log.Error("Failed to initialize JWT token maker", "error", err)
		os.Exit(1)
	}

	store := repository.NewStore(pool)
	passwordHasher := hasher.NewBcryptHasher(12)
	authService := service.NewAuthService(store, passwordHasher, tokenMaker, cfg.JWTExpiration, cfg.JWTRefreshExpiration)
	userService := service.NewUserService(store, cacheStore)
	projectService := service.NewProjectService(store, cacheStore)
	taskService := service.NewTaskService(store)
	commentService := service.NewCommentService(store)
	tagService := service.NewTagService(store, cacheStore)

	// Initialize Object Storage (MinIO / S3)
	storageClient, err := storage.NewMinIOClient(
		cfg.S3Endpoint,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3BucketName,
		cfg.S3Region,
		cfg.S3UseSSL,
	)
	var attachmentService service.AttachmentService
	if err != nil {
		log.Warn("Failed to initialize object storage client", "error", err)
	} else {
		ensureCtx, ensureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := storageClient.EnsureBucket(ensureCtx); err != nil {
			log.Warn("Failed to ensure object storage bucket exists", "bucket", cfg.S3BucketName, "error", err)
		} else {
			log.Info("Object storage initialized and bucket ready", "bucket", cfg.S3BucketName, "endpoint", cfg.S3Endpoint)
		}
		ensureCancel()
		attachmentService = service.NewAttachmentService(store, storageClient, cfg.MaxUploadSizeMB)
	}

	// 8. Initialize Asynq background worker and scheduler (if enabled)
	var taskDistributor worker.TaskDistributor
	if cfg.WorkerEnabled {
		redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
		if err != nil {
			log.Warn("Failed to parse Redis URI for background worker", "error", err)
		} else {
			distributor := worker.NewRedisTaskDistributor(redisOpt)
			taskDistributor = distributor
			defer func() {
				if err := distributor.Close(); err != nil {
					log.Warn("Error closing task distributor", "error", err)
				}
			}()

			processor := worker.NewRedisTaskProcessor(redisOpt, cfg, store, distributor, log)
			if err := processor.Start(); err != nil {
				log.Error("Failed to start background task processor", "error", err)
			} else {
				defer processor.Shutdown()
			}

			scheduler, err := worker.NewScheduler(redisOpt, cfg, log)
			if err != nil {
				log.Error("Failed to initialize background task scheduler", "error", err)
			} else {
				if err := scheduler.Start(); err != nil {
					log.Error("Failed to start background task scheduler", "error", err)
				} else {
					defer scheduler.Shutdown()
				}
			}
			log.Info("Background task worker and cron scheduler initialized successfully",
				"concurrency", cfg.WorkerConcurrency,
				"cleanup_cron", cfg.CleanupCronSchedule,
				"deadline_cron", cfg.DeadlineReminderCronSchedule,
			)
		}
	}

	webhookService := service.NewWebhookService(store, taskDistributor, cfg.AllowLocalWebhooks)
	if d, ok := taskService.(service.EventDispatchable); ok {
		d.SetEventDispatcher(webhookService)
	}
	if d, ok := commentService.(service.EventDispatchable); ok {
		d.SetEventDispatcher(webhookService)
	}
	if d, ok := attachmentService.(service.EventDispatchable); ok {
		d.SetEventDispatcher(webhookService)
	}

	// 9. Initialize HTTP router with dependencies
	router := transportHttp.NewRouter(transportHttp.RouterDependencies{
		Config:            cfg,
		Logger:            log,
		DBPool:            pool,
		Cache:             cacheStore,
		Metrics:           appMetrics,
		AuthService:       authService,
		UserService:       userService,
		ProjectService:    projectService,
		TaskService:       taskService,
		CommentService:    commentService,
		AttachmentService: attachmentService,
		WebhookService:    webhookService,
		TagService:        tagService,
		TokenMaker:        tokenMaker,
	})

	// 5. Configure HTTP server with production-grade timeouts
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           router,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	// 6. Setup signal notification context for graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in background goroutine
	go func() {
		log.Info("Server listening for incoming requests", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server encountered fatal error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for termination signal
	<-shutdownCtx.Done()

	log.Info("Termination signal received, beginning graceful shutdown...")

	// Create timeout context for connection draining
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer drainCancel()

	if err := srv.Shutdown(drainCtx); err != nil {
		log.Error("Graceful shutdown failed, forcing server stop", "error", err)
		os.Exit(1)
	}

	log.Info("Server gracefully stopped, all connections drained")
}

// maskRedisURL redacts sensitive credentials (e.g. password) from a Redis connection string.
func maskRedisURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	return u.Redacted()
}
