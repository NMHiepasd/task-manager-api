package metrics

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics encapsulates all Prometheus metrics collectors for the application.
type Metrics struct {
	Registry         *prometheus.Registry
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	ResponseSize     *prometheus.HistogramVec
	InFlightRequests prometheus.Gauge

	// Database connection pool collectors
	dbPoolTotalConns    prometheus.GaugeFunc
	dbPoolIdleConns     prometheus.GaugeFunc
	dbPoolAcquiredConns prometheus.GaugeFunc
	dbPoolMaxConns      prometheus.GaugeFunc
}

// NewMetrics initializes and registers RED (Rate, Errors, Duration) metrics with custom registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "taskmanager",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests processed, partitioned by method, path, and status code.",
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "taskmanager",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "Histogram of HTTP request latency in seconds.",
				Buckets: []float64{
					0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
				},
			},
			[]string{"method", "path", "status"},
		),
		ResponseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "taskmanager",
				Subsystem: "http",
				Name:      "response_size_bytes",
				Help:      "Histogram of HTTP response payload sizes in bytes.",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 6), // 100B, 1KB, 10KB, 100KB, 1MB, 10MB
			},
			[]string{"method", "path", "status"},
		),
		InFlightRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "taskmanager",
				Subsystem: "http",
				Name:      "in_flight_requests",
				Help:      "Current number of active HTTP requests being handled.",
			},
		),
	}

	// Register collectors
	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.ResponseSize,
		m.InFlightRequests,
	)

	return m
}

// RegisterDBPoolStats binds dynamic database connection pool metrics to the registry.
func (m *Metrics) RegisterDBPoolStats(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}

	m.dbPoolTotalConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "taskmanager",
			Subsystem: "db",
			Name:      "pool_total_conns",
			Help:      "Total number of connections in the PostgreSQL connection pool.",
		},
		func() float64 {
			return float64(pool.Stat().TotalConns())
		},
	)

	m.dbPoolIdleConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "taskmanager",
			Subsystem: "db",
			Name:      "pool_idle_conns",
			Help:      "Number of idle connections available in the PostgreSQL pool.",
		},
		func() float64 {
			return float64(pool.Stat().IdleConns())
		},
	)

	m.dbPoolAcquiredConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "taskmanager",
			Subsystem: "db",
			Name:      "pool_acquired_conns",
			Help:      "Number of currently acquired/active connections in use.",
		},
		func() float64 {
			return float64(pool.Stat().AcquiredConns())
		},
	)

	m.dbPoolMaxConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "taskmanager",
			Subsystem: "db",
			Name:      "pool_max_conns",
			Help:      "Maximum allowed connections configured for the pool.",
		},
		func() float64 {
			return float64(pool.Stat().MaxConns())
		},
	)

	m.Registry.MustRegister(
		m.dbPoolTotalConns,
		m.dbPoolIdleConns,
		m.dbPoolAcquiredConns,
		m.dbPoolMaxConns,
	)
}

// ObserveRequest records an HTTP request's execution details.
func (m *Metrics) ObserveRequest(method, path string, statusCode int, duration time.Duration, sizeBytes int) {
	statusStr := strconv.Itoa(statusCode)
	m.RequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.RequestDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())
	if sizeBytes > 0 {
		m.ResponseSize.WithLabelValues(method, path, statusStr).Observe(float64(sizeBytes))
	}
}
