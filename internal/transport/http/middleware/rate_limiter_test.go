package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/time/rate"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestIPRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Allow 1 req/sec with burst of 2
	limiter := middleware.NewIPRateLimiter(rate.Limit(1), 2)

	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(limiter.Middleware())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// First 2 requests within burst should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request should be blocked with 429 Too Many Requests
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	// Request from another IP should still succeed
	reqOther := httptest.NewRequest(http.MethodGet, "/ping", nil)
	reqOther.RemoteAddr = "10.0.0.1:54321"
	recOther := httptest.NewRecorder()
	r.ServeHTTP(recOther, reqOther)
	assert.Equal(t, http.StatusOK, recOther.Code)
}

func TestRateLimiter_FallbackToInMemory_WhenNoopCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// NoopCache will trigger the in-memory fallback
	noop := cache.NewNoopCache()
	limiter := middleware.NewRateLimiter(noop, "test", 10, time.Minute, rate.Limit(1), 2, nil)

	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(limiter.Middleware())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// First 2 requests within fallback burst succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.2.100:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request is blocked by fallback limiter
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.2.100:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

type mockFailingCache struct {
	cache.NoopCache
}

func (m *mockFailingCache) RateLimit(_ context.Context, _ string, _ int, _ time.Duration) (cache.RateLimitResult, error) {
	return cache.RateLimitResult{}, errors.New("redis connection refused")
}

func TestRateLimiter_FallbackToInMemory_OnRedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	failingCache := &mockFailingCache{}
	limiter := middleware.NewRateLimiter(failingCache, "test", 10, time.Minute, rate.Limit(1), 2, nil)

	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(limiter.Middleware())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Requests should still be protected by fallback without crashing or returning 500
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.3.100:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request blocked by fallback
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.3.100:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter_DistributedWithRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	redisCache, err := cache.NewRedisCache(ctx, "redis://localhost:6379/2", "", 2)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer redisCache.Close()

	keyPrefix := "dist-test-" + time.Now().Format("150405000")
	limiter := middleware.NewRateLimiter(redisCache, keyPrefix, 2, 2*time.Second, rate.Limit(10), 10, nil)

	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(limiter.Middleware())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	ip := "192.168.4.1"

	// Req 1: Allowed, remaining = 1
	req1 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req1.RemoteAddr = ip + ":1111"
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, "2", rec1.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "1", rec1.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec1.Header().Get("X-RateLimit-Reset"))

	// Req 2: Allowed, remaining = 0
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.RemoteAddr = ip + ":2222"
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "2", rec2.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", rec2.Header().Get("X-RateLimit-Remaining"))

	// Req 3: Blocked!
	req3 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req3.RemoteAddr = ip + ":3333"
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
	assert.Equal(t, "2", rec3.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", rec3.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec3.Header().Get("Retry-After"))

	// Clean up Redis key
	_ = redisCache.Delete(ctx, "ratelimit:"+keyPrefix+":"+ip)
}
