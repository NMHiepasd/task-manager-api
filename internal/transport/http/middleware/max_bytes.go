package middleware

import (
	"net/http"
	"task-manager-api/internal/domain"

	"github.com/gin-gonic/gin"
)

// MaxBytes returns a middleware limiting the HTTP request body size.
// If request body exceeds maxBytes, reading fails with http.MaxBytesError.
func MaxBytes(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()

		// If MaxBytesReader was exceeded, attach domain error
		if c.Writer.Status() == http.StatusRequestEntityTooLarge {
			_ = c.Error(domain.ErrPayloadTooLarge)
		}
	}
}
