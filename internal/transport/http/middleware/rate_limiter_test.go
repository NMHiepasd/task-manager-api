package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
