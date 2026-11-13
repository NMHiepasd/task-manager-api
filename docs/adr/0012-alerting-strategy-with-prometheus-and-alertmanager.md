# ADR-012: Alerting Strategy with Prometheus Alert Rules and Alertmanager

## Status
Accepted

## Context
Operating a high-reliability REST API requires proactive detection of degradation, service outages, and resource exhaustion before users are severely impacted:
1. **Critical Service Degradation**: Outages (service down) and elevated 5xx error rates must trigger critical alerts immediately.
2. **Performance Regressions**: Latency percentiles (p95) exceeding latency SLOs (e.g. > 200ms) need early warning notifications.
3. **Database Capacity Limits**: PostgreSQL connection pool exhaustion directly causes request queues and timeouts, necessitating warning thresholds (> 80% pool utilization).
4. **Declarative & Versioned**: Alert definitions must be stored in source control, validated during automated pipelines (`promtool`), and automatically provisioned without manual clicks.
5. **Deduplication & Inhibition**: Redundant alerts must be grouped, and lower-severity alerts suppressed when a higher-severity root cause is firing (e.g., inhibit latency warnings if the API is completely down).

## Decision

### 1. Prometheus Alert Rules (`deploy/prometheus/alerts.yml`)
We define declarative alerting rules evaluated every 5 seconds by Prometheus:
- **`APIDown`** (Critical):
  - Expression: `up{job="task-manager-api"} == 0`
  - Threshold: For 30 seconds
  - Description: Fires when the API instance is unreachable or fails health check probes.
- **`HighHTTPErrorRate`** (Critical):
  - Expression: `(sum(rate(taskmanager_http_requests_total{status=~"5.."}[1m])) / sum(rate(taskmanager_http_requests_total[1m]))) * 100 > 5`
  - Threshold: 5xx error rate > 5% for 1 minute
  - Description: Protects user experience and error budget against server-side crashes.
- **`HighLatencyP95`** (Warning):
  - Expression: `histogram_quantile(0.95, sum(rate(taskmanager_http_request_duration_seconds_bucket[1m])) by (le)) > 0.2`
  - Threshold: p95 latency > 200ms for 1 minute
  - Description: Alerts on query degradation, high GC pauses, or slow database responses.
- **`PostgresPoolNearExhaustion`** (Warning):
  - Expression: `(taskmanager_db_pool_acquired_conns / taskmanager_db_pool_max_conns) * 100 > 80`
  - Threshold: Acquired pool connections > 80% for 1 minute
  - Description: Warns of connection starvation before connection exhaustion leads to 503 or 504 errors.

### 2. Alertmanager Architecture (`deploy/alertmanager/alertmanager.yml`)
We deploy `prom/alertmanager:v0.27.0` on port `9093`:
- **Routing & Grouping**: Groups alerts by `['alertname', 'severity']` with `group_wait: 10s`, `group_interval: 1m`, and `repeat_interval: 1h`. Critical alerts repeat every `15m`.
- **Inhibition Rules**: Suppresses `warning` alerts if a matching `critical` alert for the same instance is already active.
- **Receivers**: Configured with dedicated receivers (`default-receiver`, `critical-receiver`) ready for Webhook, Slack, PagerDuty, or Email integrations.

### 3. Grafana Alertmanager Integration
- Registered Alertmanager as an automated datasource in Grafana (`deploy/grafana/provisioning/datasources/prometheus.yml`) with `jsonData.implementation: prometheus`.
- Operators can view, filter, silence, and inspect active alerts directly inside the Grafana UI at `http://localhost:3001/alerting`.

### 4. Continuous Verification (`promtool`)
Alert rules are verified using official `promtool check rules` and `promtool check config`, ensuring zero syntax or metric label errors reach production.

## Consequences

### Positive
- **Instant Fault Isolation**: Operators know within 30-60 seconds when critical degradation occurs.
- **Deduplication**: Alert storms are suppressed through grouping and inhibition rules.
- **Seamless Integration**: Fully unified with existing Prometheus RED metrics and Grafana dashboards.

### Negative / Trade-offs
- Adds an additional container (`alertmanager`, ~25MB Alpine image) to the local dev and production compose stack.
