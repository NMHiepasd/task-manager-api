package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"task-manager-api/internal/cache"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sampleData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestNoopCache(t *testing.T) {
	c := cache.NewNoopCache()
	ctx := context.Background()

	require.NoError(t, c.Ping(ctx))
	require.NoError(t, c.Set(ctx, "k", "v", time.Minute))

	var val string
	err := c.Get(ctx, "k", &val)
	assert.ErrorIs(t, err, cache.ErrCacheMiss)

	require.NoError(t, c.Delete(ctx, "k"))
	require.NoError(t, c.DeleteByPattern(ctx, "k*"))

	var remembered sampleData
	fetchCalled := false
	err = c.Remember(ctx, "obj", time.Minute, &remembered, func(_ context.Context) (any, error) {
		fetchCalled = true
		return sampleData{ID: "123", Name: "Test"}, nil
	})
	require.NoError(t, err)
	assert.True(t, fetchCalled)
	assert.Equal(t, "123", remembered.ID)
	assert.Equal(t, "Test", remembered.Name)

	require.NoError(t, c.Close())
}

func TestRedisCache_FullLifecycle_And_Singleflight(t *testing.T) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/1", "", 1)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer c.Close()

	// 1. Ping
	require.NoError(t, c.Ping(ctx))

	// 2. Set & Get
	key := "test:item:" + time.Now().Format("150405.000")
	original := sampleData{ID: "item-1", Name: "High Performance Item"}
	require.NoError(t, c.Set(ctx, key, original, 10*time.Second))

	var retrieved sampleData
	require.NoError(t, c.Get(ctx, key, &retrieved))
	assert.Equal(t, original, retrieved)

	// 3. Delete
	require.NoError(t, c.Delete(ctx, key))
	var miss sampleData
	err = c.Get(ctx, key, &miss)
	assert.ErrorIs(t, err, cache.ErrCacheMiss)

	// 4. DeleteByPattern
	pKey1 := "test:pattern:1"
	pKey2 := "test:pattern:2"
	require.NoError(t, c.Set(ctx, pKey1, "val1", time.Minute))
	require.NoError(t, c.Set(ctx, pKey2, "val2", time.Minute))
	require.NoError(t, c.DeleteByPattern(ctx, "test:pattern:*"))

	var temp string
	assert.ErrorIs(t, c.Get(ctx, pKey1, &temp), cache.ErrCacheMiss)
	assert.ErrorIs(t, c.Get(ctx, pKey2, &temp), cache.ErrCacheMiss)

	// 5. Anti-Stampede Singleflight Verification
	stampedeKey := "test:stampede:" + time.Now().Format("150405.000")
	var fetchCounter int32
	var wg sync.WaitGroup

	numGoroutines := 20
	results := make([]sampleData, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			var data sampleData
			remErr := c.Remember(ctx, stampedeKey, 5*time.Second, &data, func(_ context.Context) (any, error) {
				atomic.AddInt32(&fetchCounter, 1)
				time.Sleep(50 * time.Millisecond) // simulate slow database query
				return sampleData{ID: "shared", Name: "Singleflight Protected"}, nil
			})
			assert.NoError(t, remErr)
			results[idx] = data
		}()
	}

	wg.Wait()

	// CRITICAL ASSERTION: Out of 20 concurrent requests, DB fetch was executed exactly ONCE!
	assert.Equal(t, int32(1), atomic.LoadInt32(&fetchCounter), "Singleflight must eliminate cache stampede")
	for _, res := range results {
		assert.Equal(t, "shared", res.ID)
		assert.Equal(t, "Singleflight Protected", res.Name)
	}

	// Cleanup
	_ = c.Delete(ctx, stampedeKey)
}

func TestNoopCache_RateLimit(t *testing.T) {
	c := cache.NewNoopCache()
	res, err := c.RateLimit(context.Background(), "test-key", 10, time.Minute)
	assert.ErrorIs(t, err, cache.ErrNoopCache)
	assert.False(t, res.Allowed)
}

func TestRedisCache_RateLimit(t *testing.T) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/1", "", 1)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer c.Close()

	key := "ratelimit:test:" + time.Now().Format("150405.000000")
	defer func() { _ = c.Delete(ctx, key) }()

	// Limit: 3 requests per 1 second
	limit := 3
	window := 1 * time.Second

	// 1st request -> allowed, remaining 2
	res1, err := c.RateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	assert.True(t, res1.Allowed)
	assert.Equal(t, 2, res1.Remaining)

	// 2nd request -> allowed, remaining 1
	res2, err := c.RateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	assert.True(t, res2.Allowed)
	assert.Equal(t, 1, res2.Remaining)

	// 3rd request -> allowed, remaining 0
	res3, err := c.RateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	assert.True(t, res3.Allowed)
	assert.Equal(t, 0, res3.Remaining)

	// 4th request -> blocked!
	res4, err := c.RateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	assert.False(t, res4.Allowed)
	assert.Equal(t, 0, res4.Remaining)
	assert.Greater(t, res4.RetryAfter, time.Duration(0))

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// 5th request -> allowed again
	res5, err := c.RateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	assert.True(t, res5.Allowed)
	assert.Equal(t, 2, res5.Remaining)
}

func TestRedisCache_RateLimit_Concurrent(t *testing.T) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/1", "", 1)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer c.Close()

	key := "ratelimit:concurrent:" + time.Now().Format("150405.000000")
	defer func() { _ = c.Delete(ctx, key) }()

	limit := 5
	window := 2 * time.Second
	totalRequests := 20

	var allowedCount int32
	var blockedCount int32
	var wg sync.WaitGroup

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := c.RateLimit(ctx, key, limit, window)
			if err != nil {
				return
			}
			if res.Allowed {
				atomic.AddInt32(&allowedCount, 1)
			} else {
				atomic.AddInt32(&blockedCount, 1)
			}
		}()
	}

	wg.Wait()

	// Exactly 5 allowed, 15 blocked
	assert.Equal(t, int32(limit), atomic.LoadInt32(&allowedCount))
	assert.Equal(t, int32(totalRequests-limit), atomic.LoadInt32(&blockedCount))
}
