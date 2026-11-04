package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"task-manager-api/internal/config"
	"task-manager-api/internal/database"
	"task-manager-api/pkg/logger"
	"task-manager-api/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Load & validate configuration (fail-fast)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	// 2. Initialize structured JSON logger
	log := logger.Setup(cfg.LogLevel, cfg.AppEnv)
	log.Info("Starting application",
		"env", cfg.AppEnv,
		"port", cfg.AppPort,
		"log_level", cfg.LogLevel,
	)

	// 3. Connect to PostgreSQL using pgxpool
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("Successfully connected to PostgreSQL connection pool",
		"max_conns", cfg.DBMaxConns,
		"min_conns", cfg.DBMinConns,
	)

	// 4. Setup Gin engine
	if cfg.GinMode == gin.ReleaseMode || cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	// Health check endpoint (temporary for Batch 1 verification)
	router.GET("/health", func(c *gin.Context) {
		pingCtx, pingCancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer pingCancel()

		dbStatus := "connected"
		if err := pool.Ping(pingCtx); err != nil {
			dbStatus = "disconnected"
		}

		c.JSON(http.StatusOK, response.Success(gin.H{
			"status":   "ok",
			"database": dbStatus,
			"time":     time.Now().UTC(),
		}))
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           router,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	go func() {
		log.Info("Server listening for HTTP requests", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server encountered fatal error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown with signal notification
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutdown signal received, draining connections...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server exited cleanly")
}
