# ADR-010: Observability Strategy (Prometheus RED Metrics, OpenTelemetry Tracing, and Grafana)

## Status
Accepted

## Context
Operating a production REST API requires real-time visibility into traffic volume, error rates, latency percentiles, database connection health, and end-to-end request tracing across service boundaries. Specifically:
1. **RED Method**: Operators must have clear insight into Request Rate, Errors, and Duration across all endpoints.
2. **Cardinality Protection**: Metric labels must not contain high-cardinality values (such as dynamic path parameters like `/tasks/01931a74-9f7a-...` or query strings), which cause unbounded memory growth and query degradation in time-series databases.
3. **Distributed Tracing**: Requests must be traced across HTTP boundaries using standard W3C TraceContext (`traceparent` header), allowing distributed tracing when calling downstream services or async workers.
4. **Out-of-the-Box Dashboards**: Development and operations teams need zero-configuration visualization via Grafana dashboard provisioning without manual UI clicks.

## Decision

### 1. Prometheus Metrics via Dedicated Subsystem (`pkg/metrics`)
We implement standard RED metrics using the official `prometheus/client_golang` library:
- **`taskmanager_http_requests_total`** (CounterVec): Tracks total requests labeled by `method`, `path`, and `status`.
- **`taskmanager_http_request_duration_seconds`** (HistogramVec): Measures request latencies with standard web buckets (`0.005s` to `10s`), labeled by `method`, `path`, and `status`.
- **`taskmanager_http_response_size_bytes`** (HistogramVec): Measures HTTP response payload sizes.
- **`taskmanager_http_in_flight_requests`** (Gauge): Tracks currently active in-flight requests.
- **Database Connection Pool Metrics**: Dynamic gauges registered via `prometheus.NewGaugeFunc` reporting `pgxpool.Pool` stats:
  - `taskmanager_db_pool_total_conns`
  - `taskmanager_db_pool_idle_conns`
  - `taskmanager_db_pool_acquired_conns`
  - `taskmanager_db_pool_max_conns`

### 2. Cardinality Protection via Route Normalization
To prevent cardinality explosion:
- The Prometheus HTTP middleware (`internal/transport/http/middleware/metrics.go`) extracts `c.FullPath()` from the Gin context (e.g. `/api/v1/tasks/:id`) instead of `c.Request.URL.Path` (e.g. `/api/v1/tasks/12345`).
- Unmatched routes (404s) fallback to `"unmatched"`.
- Requests to the `/metrics` endpoint itself are excluded from metrics tracking to avoid self-referential scrape pollution.

### 3. OpenTelemetry Distributed Tracing (`pkg/tracer`)
We integrate OpenTelemetry SDK (`go.opentelemetry.io/otel`):
- Global `TracerProvider` configured with standard resource attributes (`service.name`, `service.version`).
- W3C TraceContext propagation set as global TextMapPropagator (`propagation.TraceContext{}`).
- Integrated via `otelgin.Middleware("task-manager-api")` in Gin router, automatically creating spans and propagating incoming `traceparent` headers.

### 4. Observability Stack & Dashboard Provisioning
- **Prometheus**: Containerized (`prom/prometheus:v2.54.1`) on port `9090`, scraping `api:8080/metrics` every 5 seconds.
- **Grafana**: Containerized (`grafana/grafana:11.2.0`) on host port `3001` (avoiding port `3000` dev conflicts).
- **Automated Provisioning**: Configured with automated datasource (`deploy/grafana/provisioning/datasources/prometheus.yml`) and pre-built operational dashboard (`deploy/grafana/provisioning/dashboards/task-manager-overview.json`) featuring:
  - Total HTTP Request Rate by status code (`sum(rate(taskmanager_http_requests_total[1m])) by (status)`)
  - Request Latency Percentiles (p50, p95, p99) using `histogram_quantile`
  - In-flight requests gauge
  - PostgreSQL connection pool utilization (acquired vs idle vs max)

## Consequences

### Positive
- **Standardized Telemetry**: Open standards (Prometheus format and OpenTelemetry W3C tracing) ensure compatibility with Grafana, Jaeger, Datadog, or GCP Cloud Trace.
- **Strict Bounded Memory**: Route pattern normalization (`c.FullPath()`) guarantees metric cardinality remains constant regardless of request volume or user-generated IDs.
- **Immediate Observability**: Developers spinning up `docker compose up` get working metrics and Grafana dashboards instantly without manual dashboard imports.

### Negative / Trade-offs
- Slight overhead (~a few microseconds per request) for histogram recording and span creation.
- Tracing currently uses standard OTel provider; export to external collectors (OTLP exporter / Tempo / Jaeger) can be attached via standard environment variables (`OTEL_EXPORTER_OTLP_ENDPOINT`).
