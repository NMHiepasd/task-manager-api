package middleware

import (
	"net/http"
	"strings"

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
	}
}

// RequestSizeLimiter dynamically limits the HTTP request body size:
// defaultLimit for standard API calls and uploadLimit for file attachment uploads.
func RequestSizeLimiter(defaultLimit, uploadLimit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultLimit
		if strings.HasSuffix(c.Request.URL.Path, "/attachments") && c.Request.Method == http.MethodPost {
			limit = uploadLimit
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
