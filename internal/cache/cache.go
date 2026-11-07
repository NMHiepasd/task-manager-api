package cache

import (
	"context"
	"errors"
	"time"
)

// ErrCacheMiss indicates that the requested key was not found in the cache store.
var (
	ErrCacheMiss = errors.New("cache: key not found")
	ErrNoopCache = errors.New("cache: rate limiting not supported on noop cache")
)

// RateLimitResult represents the evaluation result of a rate limit check.
type RateLimitResult struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	ResetAfter time.Duration
}

// Cache defines the contract for cache storage with anti-stampede protection.
type Cache interface {
	// Get retrieves an item from cache and unmarshals it into dest.
	// Returns ErrCacheMiss if key does not exist.
	Get(ctx context.Context, key string, dest any) error

	// Set serializes value into JSON and stores it with the specified TTL.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes one or more keys from the cache.
	Delete(ctx context.Context, keys ...string) error

	// DeleteByPattern removes keys matching the specified glob pattern safely using SCAN.
	DeleteByPattern(ctx context.Context, pattern string) error

	// Remember retrieves the key from cache, or executes fetch under singleflight locking
	// to prevent cache stampede, caches the result, and copies it into dest.
	Remember(ctx context.Context, key string, ttl time.Duration, dest any, fetch func(ctx context.Context) (any, error)) error

	// RateLimit checks whether key has exceeded limit within the sliding window.
	// Uses an atomic sliding window counter to prevent boundary burst anomalies.
	RateLimit(ctx context.Context, key string, limit int, window time.Duration) (RateLimitResult, error)

	// Ping checks connectivity with the underlying cache server.
	Ping(ctx context.Context) error

	// Close cleanly terminates the cache client connection.
	Close() error
}
