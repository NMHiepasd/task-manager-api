# Changelog

All notable changes to the **Task Manager REST API** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [1.2.0] - 2026-11-27

### Security
- **AES-256-GCM Webhook Secret Encryption ([ADR-0020](docs/adr/0020-transactional-outbox-pattern.md))**:
  - Implemented standalone cryptographic package `pkg/crypto` providing authenticated symmetric encryption (`AES-256-GCM`) with random 12-byte nonces generated via `crypto/rand`.
  - All webhook signing secrets are encrypted prior to database insertion; ciphertext is stored as Base64 in PostgreSQL (`webhooks.secret`).
  - Implemented zero-downtime rolling update decryption fallback: seamlessly decrypts encrypted ciphertext while transparently supporting legacy plaintext secrets.
- **Decoupled Webhook Secrets from Queue Payloads ([ADR-0020](docs/adr/0020-transactional-outbox-pattern.md))**:
  - Completely removed signing secrets from Asynq/Redis task payloads (`SendWebhookPayload`).
  - Background workers now load and decrypt the webhook secret directly from the database at execution time, eliminating risk of secret exfiltration via Redis persistence dumps or loggers.
- **Account Lockout Policy ([ADR-0021](docs/adr/0021-account-lockout-and-advanced-rbac.md))**:
  - Mitigated distributed credential stuffing and botnet brute-force attacks by enforcing a stateful account lockout: 5 consecutive failed login attempts automatically lock the user account for 15 minutes (`locked_until = NOW() + INTERVAL '15 minutes'`).
  - Standard RFC 9457 error response: returns HTTP `423 Locked` (`ACCOUNT_LOCKED`).
  - Automated Self-Healing: Accounts automatically recover after 15 minutes without requiring administrator manual intervention. Successful logins reset the failure counter to 0.
- **Timing Attack & User Enumeration Resistance**:
  - The login pipeline executes dummy bcrypt password hash comparisons for both nonexistent email addresses and currently locked accounts, guaranteeing uniform latency and eliminating timing discrepancies.
- **Advanced Role-Based Access Control (RBAC) & Separation of Duties ([ADR-0021](docs/adr/0021-account-lockout-and-advanced-rbac.md))**:
  - Introduced the `auditor` system role (`domain.RoleAuditor`) alongside `admin` and `user`.
  - Implemented capability-based middlewares `RequireAnyRole` and `RequirePermission`.
  - Auditor role is granted read-only visibility into system health, audit logs, Prometheus metrics (`GET /api/v1/system/metrics`), and webhook deliveries, while being strictly barred (HTTP 403) from modifying configurations or inspecting personal user task contents.

### Added
- **Transactional Outbox Pattern for Webhooks ([ADR-0020](docs/adr/0020-transactional-outbox-pattern.md))**:
  - Introduced relational `outbox_events` table (migration `000010_create_outbox_events`) to record domain events inside the identical ACID transaction as the business operation, completely eliminating the dual-write problem and phantom events.
  - Built robust background polling relay worker `internal/worker/outbox_relay.go` utilizing PostgreSQL concurrency-safe `SELECT ... FOR UPDATE SKIP LOCKED` batch queries.
  - Added dead-letter failure handling: automatically increments event retry counts and transitions persistent errors to `status = 'failed'` after 5 attempts.
  - Added periodic retention cleanup cron task (`task:cleanup_old_outbox_events` running `@every 6h`) that prunes processed outbox events older than 7 days.
- **Database Migrations**:
  - `000010_create_outbox_events`: Schema migration and partial index for transactional outbox pattern.
  - `000011_add_account_lockout_to_users`: Added `failed_login_attempts`, `locked_until`, `last_failed_login_at` to `users` and expanded role constraint to include `auditor`.
- **Architecture Decision Records**:
  - **[ADR-0020](docs/adr/0020-transactional-outbox-pattern.md)**: Transactional Outbox Pattern for Reliable Webhook Event Dispatching.
  - **[ADR-0021](docs/adr/0021-account-lockout-and-advanced-rbac.md)**: Account Lockout Policy and Advanced Role-Based Access Control (RBAC).

### Changed
- Refactored core domain services (`TaskService`, `CommentService`, `AttachmentService`, `ProjectService`) to persist domain events into `outbox_events` within `store.WithTx`, removing immediate synchronous Asynq enqueue calls from HTTP request lifecycles.
- Re-architected background worker startup in `cmd/api/main.go` to orchestrate Asynq processor, scheduler, and OutboxRelay simultaneously with graceful shutdown synchronization.

---

## [1.1.1] - 2026-11-27

### Fixed
- **Docker Compose Production Image Tag**: Enforced `${IMAGE_TAG:?}` in `docker-compose.prod.yml` to prevent accidental rollbacks or running unversioned stale images.
- **MinIO Gateway Reverse Proxy Route**: Hardened Caddy reverse proxy routing for object storage (`/task-attachments/*`), explicitly restricting public HTTP requests to `GET` only (blocking unauthorized `PUT`, `DELETE`, `POST`) while ensuring seamless streaming and download.
- **Project Member Role Demotion & Removal Transaction**: Wrapped project member role updates (`UpdateMemberRole`) and member removals (`RemoveMember`) in atomic database transactions (`store.ExecTx`), ensuring project member deletion, role modification, and task unassignment occur atomically before cache invalidation.
- **Atomic Attachment Quota Reservation**: Eliminated TOCTOU race conditions under parallel file uploads by implementing PostgreSQL transaction-level advisory locks (`pg_advisory_xact_lock`) with deterministic ordering (`user_id -> task_id`). Added resilient orphan cleanup fallback if subsequent database recording fails.

### Changed
- **Webhook Management & Access Policy ([ADR-0019](docs/adr/0019-webhook-management-policy.md))**:
  - Enforced project webhook authorization matrix in `authorizeWebhookAccess`: project-scoped webhooks are strictly manageable by project owners only (editors and viewers are denied access).
  - Anti-IDOR enforcement: unauthorized or cross-project webhook lookup and mutation attempts consistently return RFC 9457 `404 Not Found` (never 403).
  - Inactive webhooks of removed or demoted members remain disabled and can only be managed, viewed, or deleted by project owners.
- **Deprecated Webhook Signature Cleanup**:
  - Removed obsolete legacy signature helper functions `ComputeSignature` and `VerifySignature` from `pkg/webhook`, consolidating exclusively on the Webhook V2 signing standard (`SignV2` and `VerifyV2`).

### Security
- **Go Toolchain Pinning & Scanner Hardening**:
  - Pinned Go toolchain to `go1.27.4` in `go.mod` (`toolchain go1.27.4`) and Dockerfile builder stage (`golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280`).
  - Resolved standard library advisories `GO-2026-6859` (HTTP/1 server crash in `net/http`) and `GO-2026-6809` (intermediate CA distrust in `crypto/x509`), achieving **0 vulnerabilities** in `govulncheck ./...`.
  - Pinned security scanner versions across CI and reproduction documentation: `gosec@v2.30.0`, `gitleaks:v8.24.0`, `trivy:0.76.2`.
  - Added CI Go version verification (`go-version-file: 'go.mod'` + `Verify Go Version`) across all workflows.

---

## [1.1.0] - 2026-11-18

### Breaking Changes
- **User Privacy Hardening (Task Comments & Webhooks)**:
  - Removed `email` field from `CommentResponse` DTO (`GET /api/v1/tasks/{id}/comments`, `POST /api/v1/tasks/{id}/comments`, `PATCH /api/v1/tasks/{id}/comments/{commentId}`).
  - Removed `email` field from `comment.created` webhook event data payload. Only non-sensitive public attributes (`author` display name, `user_id` UUID) are exposed to project collaborators and webhook subscribers.
- **Webhook V2 Signature & Anti-Replay Architecture**:
  - Deprecated legacy `X-Webhook-Signature: sha256=<hex>` header.
  - Upgraded all deliveries to **Webhook V2** standard:
    - Added `X-Webhook-Id: <delivery_uuid>` (stable across all retries of the event).
    - Added `X-Webhook-Timestamp: <unix_seconds>` (fresh timestamp generated per attempt).
    - Added `X-Webhook-Signature: v1=<hmac_hex>` computed over `<timestamp>.<delivery_id>.<raw_body_bytes>`.
    - Added `id` field to the top-level webhook event envelope JSON.
  - **Strict Secret Policy**: User-supplied webhook secrets must now be at least 32 characters (auto-generates 24-byte crypto random string if omitted).
  - **Write-Only Secret Visibility**: Webhook secrets are returned only once upon creation (`POST /api/v1/webhooks`) and masked (`""` / omitted) in `GET`, `LIST`, and `PATCH` endpoints.

### Added
- **Centralized Authorization & Task Access Policy (ADR-0018)**:
  - Introduced pure functional authorization package `internal/authz` implementing the comprehensive permission matrix defined in [ADR-0018](docs/adr/0018-task-access-policy.md).
  - Integrated `authz.Can` across all service layers: `TaskService`, `CommentService`, and `AttachmentService`.
  - Added project collaborator cleanup: removing a member automatically resets assigned tasks (`assignee_id = NULL`) and deactivates their project webhooks.
  - Anti-IDOR enforcement: unauthorized access attempts return RFC 9457 `404 Not Found` (never 403) to prevent resource existence enumeration.
  - Comprehensive live database integration test matrix in `internal/service/authz_matrix_integration_test.go` using Testcontainers.
- **Webhook V2 Verification Library**:
  - Introduced `pkg/webhook.SignV2` and `pkg/webhook.VerifyV2` with constant-time comparison and 5-minute configurable timestamp tolerance window.
  - Added sentinel errors: `ErrSignatureMismatch`, `ErrTimestampExpired`, `ErrInvalidSignatureFormat`, `ErrInvalidTimestampFormat`.
  - Integration guide with Go, Node.js, and Python verification code in [`docs/webhooks.md`](docs/webhooks.md).
- **Webhook Delivery Lifecycle Hardening**:
  - Configurable `WEBHOOK_MAX_RETRY` (env, default: 6) with 15s task execution timeout.
  - Sentinel error `webhook.ErrBlockedIP` for anti-SSRF dialer rejections, replacing string-based matching in worker processor.
  - Automatic deactivation of project webhooks (`DeactivateProjectWebhooksByUser`) when project collaborators are removed or demoted to `viewer`.
  - Restricted `ListActiveWebhooksForEvent` to ensure project webhooks strictly dispatch events to active project owners/editors.
- **Attachment Quotas & Secure Download**:
  - Enforced storage limits via `MAX_ATTACHMENTS_PER_TASK` (default 20) and `MAX_USER_STORAGE_MB` (default 500MB).
  - Safe attachment download streaming using `mime.FormatMediaType` and `X-Content-Type-Options: nosniff`.
  - Resilient orphan cleanup: failed `storage.Delete` calls enqueue background retry tasks (`storage:delete_object`).
- **Production Hardening & CI/CD Enhancements**:
  - Strict configuration startup validation for `APP_ENV`, `GIN_MODE=release`, `REDIS_PASSWORD`, and `RequestTimeout < InProgressLockTTL`.
  - Race-safe idempotency handling: conditional stale lock deletion using `locked_at` and HTTP 409 `IDEMPOTENCY_IN_PROGRESS` on race collisions.
  - Base container images pinned by immutable SHA-256 digests across all Compose configurations.
  - Full Git history secret scanning via Gitleaks action.
  - Code coverage threshold enforcement ($\ge 65\%$ on service layer) and artifact report uploads.
  - Production smoke test automation scripts for POSIX Bash (`scripts/smoke-prod.sh`) and Windows PowerShell (`scripts/smoke-prod.ps1`).

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
