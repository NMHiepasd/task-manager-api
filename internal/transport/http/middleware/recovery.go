package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"task-manager-api/internal/domain"

	"github.com/gin-gonic/gin"
)

// Recovery returns a Gin middleware that recovers from panics and returns an RFC 9457 error.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				reqID := GetRequestID(c)
				stack := string(debug.Stack())

				logger.Error("Panic recovered during HTTP request",
					"error", r,
					"stack", stack,
					"request_id", reqID,
					"path", c.Request.URL.Path,
				)

				problem := domain.NewProblem(
					http.StatusInternalServerError,
					"An unexpected internal error occurred",
					"INTERNAL_SERVER_ERROR",
					c.Request.URL.Path,
					reqID,
				)

				c.Header("Content-Type", "application/problem+json")
				c.AbortWithStatusJSON(http.StatusInternalServerError, problem)
			}
		}()
		c.Next()
	}
}
