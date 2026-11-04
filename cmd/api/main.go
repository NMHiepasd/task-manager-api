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
	"task-manager-api/internal/handler"
	"task-manager-api/pkg/logger"
	"time"
)

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

	// 4. Initialize HTTP router with dependencies
	router := handler.NewRouter(handler.RouterDependencies{
		Config: cfg,
		Logger: log,
		DBPool: pool,
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
