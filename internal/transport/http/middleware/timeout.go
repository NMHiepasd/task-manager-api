package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout returns a Gin middleware that associates a timeout deadline with the request context.
// When the deadline expires, the context is cancelled, automatically terminating downstream
// database queries and context-aware I/O operations.
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if timeout <= 0 {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
