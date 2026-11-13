package metrics_test

import (
	"task-manager-api/pkg/metrics"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_NewAndObserve(t *testing.T) {
	m := metrics.NewMetrics()
	require.NotNil(t, m)
	require.NotNil(t, m.Registry)

	// Observe a sample request
	m.ObserveRequest("GET", "/api/v1/tasks", 200, 15*time.Millisecond, 1024)

	metricFamilies, err := m.Registry.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, metricFamilies)

	foundRequestMetric := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "taskmanager_http_requests_total" {
			foundRequestMetric = true
			assert.Equal(t, 1, len(mf.GetMetric()))
			assert.Equal(t, float64(1), mf.GetMetric()[0].GetCounter().GetValue())
		}
	}
	assert.True(t, foundRequestMetric, "Expected to find taskmanager_http_requests_total metric")
}
