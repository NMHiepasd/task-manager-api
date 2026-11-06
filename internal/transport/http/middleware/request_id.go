package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDHeader is the HTTP header key for tracing requests.
	RequestIDHeader = "X-Request-ID"
	// RequestIDKey is the gin context key for the correlation ID.
	RequestIDKey = "request_id"
)

// RequestID returns a Gin middleware that extracts or generates a unique correlation ID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			newID, err := uuid.NewV7()
			if err != nil {
				reqID = uuid.NewString()
			} else {
				reqID = newID.String()
			}
		}

		c.Set(RequestIDKey, reqID)
		c.Header(RequestIDHeader, reqID)
		c.Next()
	}
}

// GetRequestID retrieves the correlation ID from context, or empty string.
func GetRequestID(c *gin.Context) string {
	if val, exists := c.Get(RequestIDKey); exists {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return c.GetHeader(RequestIDHeader)
}
