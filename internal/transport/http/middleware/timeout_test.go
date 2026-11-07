package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTimeout_CompletesInTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Timeout(50 * time.Millisecond))
	router.GET("/fast", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTimeout_ExpiresContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Timeout(20 * time.Millisecond))
	router.Use(middleware.ErrorHandler(nil))

	router.GET("/slow", func(c *gin.Context) {
		select {
		case <-time.After(100 * time.Millisecond):
			c.JSON(http.StatusOK, gin.H{"status": "too_slow"})
		case <-c.Request.Context().Done():
			_ = c.Error(c.Request.Context().Err())
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.Contains(t, rec.Body.String(), "GATEWAY_TIMEOUT")
}
