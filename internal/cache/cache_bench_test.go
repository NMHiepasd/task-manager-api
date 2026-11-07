package cache_test

import (
	"context"
	"task-manager-api/internal/cache"
	"testing"
	"time"
)

func BenchmarkRedisCache_GetHit(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/3", "", 3)
	if err != nil {
		b.Skip("Skipping Redis benchmark: local redis not available")
		return
	}
	defer c.Close()

	key := "bench:get:hit"
	original := sampleData{ID: "bench-1", Name: "Benchmark Sample Data"}
	if err := c.Set(ctx, key, original, 5*time.Minute); err != nil {
		b.Fatalf("failed to seed cache: %v", err)
	}
	defer func() { _ = c.Delete(ctx, key) }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var dest sampleData
		_ = c.Get(ctx, key, &dest)
	}
}

func BenchmarkRedisCache_RateLimit(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/3", "", 3)
	if err != nil {
		b.Skip("Skipping Redis benchmark: local redis not available")
		return
	}
	defer c.Close()

	key := "bench:ratelimit"
	defer func() { _ = c.Delete(ctx, key) }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = c.RateLimit(ctx, key, 1000000, time.Minute)
	}
}
