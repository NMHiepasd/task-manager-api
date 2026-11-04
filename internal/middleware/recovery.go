package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"task-manager-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// Recovery recovers from any panics and writes a 500 error response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}

				stack := string(debug.Stack())
				log.Error("Panic recovered in HTTP handler",
					"error", err,
					"stack", stack,
					"path", c.Request.URL.Path,
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, response.Error(
					"INTERNAL_SERVER_ERROR",
					"An unexpected internal error occurred",
				))
			}
		}()
		c.Next()
	}
}
