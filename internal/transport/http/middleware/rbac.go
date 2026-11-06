package middleware

import (
	"task-manager-api/internal/domain"

	"github.com/gin-gonic/gin"
)

// RequireRole verifies that the authenticated user possesses one of the allowed roles.
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	rolesMap := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		rolesMap[r] = true
	}

	return func(c *gin.Context) {
		payload, err := GetAuthPayload(c)
		if err != nil {
			_ = c.Error(domain.ErrUnauthorized)
			c.Abort()
			return
		}

		if !rolesMap[payload.Role] {
			_ = c.Error(domain.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}
