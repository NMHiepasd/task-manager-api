package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/token"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	maker, _ := token.NewJWTMaker("12345678901234567890123456789012")

	adminToken, _, _ := maker.CreateAccessToken(uuid.New(), "admin@example.com", "admin", time.Minute)
	userToken, _, _ := maker.CreateAccessToken(uuid.New(), "user@example.com", "user", time.Minute)

	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(middleware.AuthMiddleware(maker))

	r.GET("/admin-only", middleware.RequireRole("admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	t.Run("Authorized_Admin_Passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Forbidden_User_Blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}
