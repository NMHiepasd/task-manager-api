# Release Notes - Task Manager REST API v1.0.0

🎉 We are pleased to announce the official **v1.0.0** release of **Task Manager REST API**!

This release represents a production-ready, highly observable, and securely hardened RESTful service built with Go (Gin), PostgreSQL (pgx/v5 & sqlc), Redis (Asynq), MinIO (S3), Docker Compose, and Caddy.

---

## 🚀 Key Highlights & Features

1. **Enterprise Task Management**:
   - Comprehensive task lifecycle management with priorities, status tracking, deadlines, and project grouping.
   - High-performance keyset (cursor) pagination and bilingual (EN/VI) full-text search tuned for 100k+ records.
2. **Project Collaboration & RBAC**:
   - Multi-tenant workspace sharing with `owner`, `editor`, and `viewer` role privileges.
   - Strict project membership verification on task assignment.
3. **Task Comments & Discussion**:
   - Threaded commentary and audit trails with author-level modification protections.
4. **S3 Object Storage (MinIO)**:
   - File attachment support backed by MinIO/S3 with real MIME sniffing (`http.DetectContentType`) and strict type allowlists.
5. **Asynchronous Background Processing (Redis & Asynq)**:
   - Automated deadline notification scanner.
   - SSRF-hardened webhook dispatcher with cryptographic HMAC-SHA256 request signatures.
   - Background cleanup workers for expired tokens and stale idempotency keys.
6. **Full-Stack Observability**:
   - Prometheus metrics at `/metrics` and distributed tracing via OpenTelemetry & Jaeger.
   - Declarative alerting rules paired with Alertmanager.
7. **Standards-Compliant API**:
   - RFC 9457 Problem Details error responses.
   - IETF Idempotency-Key support with dual short/long TTL and fail-safe rollback.
   - Interactive Swagger UI documentation protected by HTTP Basic Auth in production.

---

## 🛡️ Security Hardening & Bug Fixes

- **Login Timing Attacks**: Pre-computed dynamic bcrypt dummy hash (cost 12 in production) ensures identical response time for existing vs non-existent accounts.
- **JWT Protection**: Enforced expiration claim requirement (`jwt.WithExpirationRequired()`) and environment-driven `JWT_ISSUER` & `JWT_AUDIENCE` validation.
- **Network Boundaries**: Confined trusted proxy resolution exclusively to the static Caddy reverse proxy IP (`172.28.0.2`), preventing IP spoofing.
- **Fail-Fast Startup Validation**: Halts application launch immediately if insecure default JWT secrets or `minioadmin` credentials are used in production.
- **SSRF Defense**: Blocked all private (RFC 1918), loopback, link-local, and Carrier-Grade NAT (CGNAT) IPs from webhook destinations.
- **Redis Security**: Mandatory password authentication (`--requirepass`) across runtime and healthchecks.

---

## 📦 Container & Infrastructure Upgrades

- Caddy reverse proxy pinned to `caddy:2.8-alpine` with fixed IPv4 `172.28.0.2`.
- MinIO object storage pinned to `minio/minio:RELEASE.2024-11-07T00-52-28Z`.
- GitHub Actions CI supports dual semver tag patterns (`v1.0.0` and `1.0.0`) targeting Docker image packaging.

---

## 📋 Docker Quickstart

```bash
# Clone the repository
git clone https://github.com/NMHiepasd/task-manager-api.git
cd task-manager-api

# Configure production environment
cp .env.example .env
# Ensure JWT_SECRET, DB_PASSWORD, REDIS_PASSWORD, and S3_SECRET_KEY are set to strong random values

# Start the full production stack
docker compose -f docker-compose.prod.yml up -d --build
```
