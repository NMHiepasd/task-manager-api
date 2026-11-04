package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	HeaderRequestID     = "X-Request-ID"
	ContextKeyRequestID = "request_id"
)

// RequestID attaches a unique UUID to each incoming HTTP request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
		}

		c.Set(ContextKeyRequestID, reqID)
		c.Header(HeaderRequestID, reqID)
		c.Next()
	}
}
