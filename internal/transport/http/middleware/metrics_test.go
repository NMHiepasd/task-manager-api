package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/metrics"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusMiddleware_NormalizesPathAndObserves(t *testing.T) {
	gin.SetMode(gin.TestMode)

	m := metrics.NewMetrics()
	r := gin.New()
	r.Use(middleware.Prometheus(m))

	r.GET("/api/v1/tasks/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "task detail")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/12345", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	metricFamilies, err := m.Registry.Gather()
	require.NoError(t, err)

	var foundPath string
	for _, mf := range metricFamilies {
		if mf.GetName() == "taskmanager_http_requests_total" {
			for _, metric := range mf.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "path" {
						foundPath = label.GetValue()
					}
				}
			}
		}
	}

	// Must be normalized route template, NOT the actual parameter value
	assert.Equal(t, "/api/v1/tasks/:id", foundPath)
}
