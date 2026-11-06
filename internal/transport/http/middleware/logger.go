package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogger returns a Gin middleware logging HTTP requests as structured JSON.
// Probe endpoints such as /healthz and /readyz are ignored to reduce log noise.
func StructuredLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip health check endpoints to prevent log bloat
		if path == "/healthz" || path == "/readyz" {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		status := c.Writer.Status()
		reqID := GetRequestID(c)

		attrs := []slog.Attr{
			slog.String("request_id", reqID),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", c.ClientIP()),
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		logger.LogAttrs(c.Request.Context(), level, "HTTP request handled", attrs...)
	}
}
