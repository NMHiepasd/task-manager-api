package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides all functions to execute SQL queries and transactions.
type Store interface {
	Querier
	WithTx(ctx context.Context, fn func(q Querier) error) error
}

// SQLStore implements Store interface with connection pool support.
type SQLStore struct {
	*Queries
	pool *pgxpool.Pool
}

// NewStore creates a new SQLStore instance.
func NewStore(pool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries: New(pool),
		pool:    pool,
	}
}

// WithTx executes a callback function within a database transaction.
func (s *SQLStore) WithTx(ctx context.Context, fn func(q Querier) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	qtx := s.Queries.WithTx(tx)
	if err := fn(qtx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction failed: %w", err)
	}
	return nil
}
