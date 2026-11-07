package database

import (
	"context"
	"fmt"
	"task-manager-api/internal/config"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool initializes and returns a pgx connection pool based on configuration.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Pool sizing and connection lifetime settings
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.DBHealthCheckPeriod

	// Configure server-side statement timeout to prevent runaway queries
	if cfg.DBStatementTimeout > 0 {
		if poolConfig.ConnConfig.RuntimeParams == nil {
			poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
		}
		poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", cfg.DBStatementTimeout.Milliseconds())
	}

	// Establish connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity with a quick ping
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
