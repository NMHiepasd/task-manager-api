package middleware

import (
	"task-manager-api/pkg/metrics"
	"time"

	"github.com/gin-gonic/gin"
)

// Prometheus returns a Gin middleware that records RED metrics for HTTP requests.
func Prometheus(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bypass high-frequency diagnostic/probe endpoints from request metrics
		rawPath := c.Request.URL.Path
		if rawPath == "/metrics" || rawPath == "/healthz" || rawPath == "/readyz" {
			c.Next()
			return
		}

		m.InFlightRequests.Inc()
		defer m.InFlightRequests.Dec()

		start := time.Now()

		c.Next()

		// Use normalized route template (e.g. /api/v1/tasks/:id) to prevent cardinality explosion
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		duration := time.Since(start)
		statusCode := c.Writer.Status()
		size := c.Writer.Size()

		m.ObserveRequest(c.Request.Method, path, statusCode, duration, size)
	}
}
