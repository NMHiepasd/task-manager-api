package middleware

import (
	"fmt"
	"net/http"
	"task-manager-api/internal/domain"

	"github.com/gin-gonic/gin"
)

// RequireAnyRole ensures that the authenticated caller holds at least one of the permitted system roles.
func RequireAnyRole(allowedRoles ...string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowedSet[r] = true
	}

	return func(c *gin.Context) {
		roleVal, exists := c.Get(AuthorizationUserRoleKey)
		if !exists {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusUnauthorized, domain.NewProblem(
				http.StatusUnauthorized,
				"Authentication required",
				"UNAUTHORIZED",
				c.Request.URL.Path,
				GetRequestID(c),
			))
			return
		}

		userRole, ok := roleVal.(string)
		if !ok || !allowedSet[userRole] {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusForbidden, domain.NewProblem(
				http.StatusForbidden,
				"You do not have the required role to access this resource",
				"FORBIDDEN",
				c.Request.URL.Path,
				GetRequestID(c),
			))
			return
		}

		c.Next()
	}
}

// RequireRole is a convenience helper requiring exactly one specified system role.
func RequireRole(role string) gin.HandlerFunc {
	return RequireAnyRole(role)
}

// RequirePermission verifies that the caller's system role possesses the target capability scope.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get(AuthorizationUserRoleKey)
		if !exists {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusUnauthorized, domain.NewProblem(
				http.StatusUnauthorized,
				"Authentication required",
				"UNAUTHORIZED",
				c.Request.URL.Path,
				GetRequestID(c),
			))
			return
		}

		userRole, ok := roleVal.(string)
		if !ok || !domain.HasPermission(userRole, permission) {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusForbidden, domain.NewProblem(
				http.StatusForbidden,
				fmt.Sprintf("Permission '%s' is required to access this resource", permission),
				"FORBIDDEN",
				c.Request.URL.Path,
				GetRequestID(c),
			))
			return
		}

		c.Next()
	}
}
