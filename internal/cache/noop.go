package cache

import (
	"context"
	"encoding/json"
	"time"
)

// NoopCache provides a transparent fallback implementation of Cache where all operations pass through.
type NoopCache struct{}

// NewNoopCache constructs a NoopCache.
func NewNoopCache() *NoopCache {
	return &NoopCache{}
}

// Get always returns ErrCacheMiss.
func (n *NoopCache) Get(_ context.Context, _ string, _ any) error {
	return ErrCacheMiss
}

// Set performs no operation.
func (n *NoopCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}

// Delete performs no operation.
func (n *NoopCache) Delete(_ context.Context, _ ...string) error {
	return nil
}

// DeleteByPattern performs no operation.
func (n *NoopCache) DeleteByPattern(_ context.Context, _ string) error {
	return nil
}

// Remember executes the fetch callback directly and marshals/unmarshals into dest.
func (n *NoopCache) Remember(ctx context.Context, _ string, _ time.Duration, dest any, fetch func(ctx context.Context) (any, error)) error {
	val, err := fetch(ctx)
	if err != nil {
		return err
	}

	data, err := json.Marshal(val)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Ping always succeeds.
func (n *NoopCache) Ping(_ context.Context) error {
	return nil
}

// Close performs no operation.
func (n *NoopCache) Close() error {
	return nil
}
