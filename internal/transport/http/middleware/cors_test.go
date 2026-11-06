package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allowedOrigins := []string{"http://localhost:3000", "https://example.com"}

	newRouter := func() *gin.Engine {
		r := gin.New()
		r.Use(middleware.CORS(allowedOrigins))
		r.GET("/api/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		r.POST("/api/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		return r
	}

	t.Run("AllowedOrigin_Matches", func(t *testing.T) {
		r := newRouter()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("DisallowedOrigin_NoCORSHeaders", func(t *testing.T) {
		r := newRouter()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Origin", "http://evil.com")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("Preflight_Options_Returns204", func(t *testing.T) {
		r := newRouter()
		req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	})
}
