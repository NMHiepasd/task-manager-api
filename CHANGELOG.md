# Changelog

All notable changes to the **Task Manager REST API** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2026-11-16

### Added
- **Core Task Management**:
  - Complete CRUD for tasks with priority, status lifecycle, deadlines, and project assignment.
  - High-performance keyset (cursor-based) pagination and English/Vietnamese full-text search powered by PostgreSQL GIN and `websearch_to_tsquery`.
- **Project Collaboration & RBAC**:
  - Project workspace sharing with fine-grained role-based permissions (`owner`, `editor`, `viewer`).
  - Strict membership validation ensuring task assignees belong to the respective project.
- **Task Comments & Audit Trail**:
  - Nested task comments with moderation and update/delete authorization controls.
- **File Attachments (MinIO / S3)**:
  - File attachment upload and download integrated with AWS S3-compatible MinIO object storage.
  - Strict MIME type allowlist enforcement and content sniffing via `http.DetectContentType` to prevent polyglot file execution.
- **Asynchronous Background Processing (Redis & Asynq)**:
  - Asynchronous task deadline email reminder scanner.
  - Webhook dispatcher with HMAC-SHA256 signature verification (`X-Webhook-Signature`).
  - Automated recurring cleanup jobs for expired refresh tokens and 24-hour completed idempotency records.
- **Observability & Operational Metrics**:
  - Prometheus `/metrics` endpoint with HTTP request duration and error rate counters.
  - OpenTelemetry (OTel) distributed tracing integrated with Jaeger.
  - Declarative Prometheus alerting rules paired with Alertmanager notifications.
- **API Documentation & Quality Assurance**:
  - Auto-generated Swagger UI / OpenAPI 3.0 documentation with HTTP Basic Auth protection in production environments.
  - Automated API contract and property fuzz testing suite using Schemathesis.

### Security & Hardening
- **Timing Attack Mitigation**: Dynamic dummy bcrypt hash computed from the configured hasher (cost 12 in production) to ensure constant-time response for non-existent users during login.
- **JWT Parser Hardening**:
  - Enforced `jwt.WithExpirationRequired()` to reject tokens without an `exp` claim.
  - Added configurable `JWT_ISSUER` and `JWT_AUDIENCE` options.
  - Masked internal parser errors to avoid information leakage in `AuthMiddleware`.
- **Strict Trusted Proxies**: Restricted reverse proxy trust to the static Caddy container IP (`172.28.0.2,127.0.0.1`), preventing client IP spoofing via manipulated `X-Forwarded-For` headers.
- **Fail-Fast Production Validation**:
  - Strict validation at startup: halts immediately if default/insecure JWT secrets or `minioadmin` S3 credentials are detected in production.
  - Rejects CORS configurations with wildcard `*` or `localhost`/`127.0.0.1` origins in production.
  - Enforces `ServerWriteTimeout >= RequestTimeout` alignment.
- **SSRF Protection**: Hardened webhook HTTP transport blocking loopback, link-local, RFC 1918 private subnets, and Carrier-Grade NAT (CGNAT) ranges via custom socket dialer with redirects disabled.
- **Redis Authentication**: Added `--requirepass` enforcement and authenticated `redis-cli ping` healthcheck in production container compose.

### Fixed & Enhanced
- **Idempotency Dual TTL & Rollback**:
  - Separated short in-progress lock TTL (60s) from long terminal response TTL (24h).
  - Added automatic takeover of abandoned/stale reservation locks caused by crashes or timeouts.
  - Implemented immediate rollback (`DEL key`) if the request handler fails or response persistence encounters a database error, allowing immediate client retry without 409 Conflict.
- **Database Index Optimization**: Tuned composite indexes (Migration `000008`) for keyset pagination and full-text search, achieving sub-5ms query times under 100k task load.
- **HTTP Server Timeout Alignment**: Aligned request timeout (`30s`) with write timeout (`60s`), guaranteeing standard RFC 9457 `504 Gateway Timeout` problem responses.
- **Container Infrastructure**:
  - Pinned MinIO to `minio/minio:RELEASE.2024-11-07T00-52-28Z` and Caddy to `caddy:2.8-alpine`.
  - Configured static container IP `172.28.0.2` in `docker-compose.prod.yml`.
  - Synchronized dual semver tag patterns (`v{{version}}` and `{{version}}`) in GitHub Actions build workflows with `${IMAGE_TAG:-1.0.0}` fallback.
