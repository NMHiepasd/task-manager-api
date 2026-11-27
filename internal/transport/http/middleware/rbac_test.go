package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/transport/http/middleware"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireAnyRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("MissingAuthRoleReturns401", func(t *testing.T) {
		router := gin.New()
		router.GET("/test", middleware.RequireAnyRole(domain.RoleAdmin), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("DisallowedRoleReturns403", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleUser)
			c.Next()
		})
		router.GET("/admin-only", middleware.RequireAnyRole(domain.RoleAdmin), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var prob domain.ProblemDetails
		err := json.Unmarshal(rec.Body.Bytes(), &prob)
		require.NoError(t, err)
		assert.Equal(t, "FORBIDDEN", prob.Code)
	})

	t.Run("AuditorBlockedFromUserWriteRoutes", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleAuditor)
			c.Next()
		})
		router.POST("/projects", middleware.RequireAnyRole(domain.RoleAdmin, domain.RoleUser), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/projects", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("AllowedRoleReturns200", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleAdmin)
			c.Next()
		})
		router.GET("/admin-or-auditor", middleware.RequireAnyRole(domain.RoleAdmin, domain.RoleAuditor), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/admin-or-auditor", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AdminHasAllPermissions", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleAdmin)
			c.Next()
		})
		router.GET("/metrics", middleware.RequirePermission(domain.PermissionSystemMetrics), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("AuditorHasSystemMetricsPermission", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleAuditor)
			c.Next()
		})
		router.GET("/metrics", middleware.RequirePermission(domain.PermissionSystemMetrics), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("UserLacksSystemMetricsPermission", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleUser)
			c.Next()
		})
		router.GET("/metrics", middleware.RequirePermission(domain.PermissionSystemMetrics), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("AuditorLacksTaskWritePermission", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserRoleKey, domain.RoleAuditor)
			c.Next()
		})
		router.POST("/tasks", middleware.RequirePermission(domain.PermissionTasksWrite), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}
