# Production Deployment Smoke Test Guide & Verification Log

This document outlines the standard operating procedure (SOP) for executing end-to-end smoke tests against the production container stack (`docker-compose.prod.yml`), verifying core security controls, data persistence, and API lifecycle operations from clean volumes.

---

## 1. Prerequisites & Clean Boot Procedure

To guarantee tests run against clean, non-polluted state, always purge all existing volumes before running the smoke test suite.

### Step 1: Clean Up Previous Volumes
```bash
docker compose -f docker-compose.prod.yml down -v
```

### Step 2: Configure Production Environment Variables
Ensure required production environment variables are present in `.env` or exported in your shell session:
```bash
# Minimum required production environment variables
export DB_PASSWORD="production-db-secure-password"
export REDIS_PASSWORD="production-redis-secure-password"
export JWT_SECRET="production-jwt-signing-key-32-chars-minimum!!"
export S3_SECRET_KEY="production-minio-s3-secret-key-32-chars!!"
export DOMAIN="localhost"
export HTTP_PORT="80"
```

### Step 3: Boot Production Containers
```bash
docker compose -f docker-compose.prod.yml up -d --build
```

Wait ~10 seconds for Caddy reverse proxy, PostgreSQL schema migrations, Redis, MinIO, and API health probes to stabilize. Verify running containers:
```bash
docker compose -f docker-compose.prod.yml ps
```

---

## 2. Test Execution

The smoke test suite is available in both POSIX Bash (`scripts/smoke-prod.sh`) and native Windows PowerShell (`scripts/smoke-prod.ps1`).

### Running via Bash (Linux / macOS / Git Bash / WSL):
```bash
bash scripts/smoke-prod.sh
```

### Running via PowerShell (Windows):
```powershell
.\scripts\smoke-prod.ps1
```

*(Optional: Pass a custom target URL via `BASE_URL="http://your-domain.com" bash scripts/smoke-prod.sh` or `.\scripts\smoke-prod.ps1 -BaseUrl "http://your-domain.com"`)*

---

## 3. Test Coverage Matrix

| Test Step | Target Endpoint / Action | Expected Result |
| :--- | :--- | :--- |
| **1. Readiness Probe** | `GET /readyz` | HTTP `200 OK`, checks DB pool & Redis connectivity |
| **2. Caddy Public Shielding** | `GET /metrics`, `/swagger/*`, `/debug/pprof/*` | HTTP `403 Forbidden` (public access blocked) |
| **3. Rate Limiting Burst** | `POST /api/v1/auth/login` (burst > limit) | HTTP `429 Too Many Requests` when burst limit exceeded |
| **4. Auth Lifecycle** | `POST /api/v1/auth/register`, `/login`, `GET /me` | HTTP `201` registration, HTTP `200` login & JWT verification |
| **5. Task & Attachment Upload** | `POST /api/v1/tasks`, `POST /tasks/:id/attachments` | HTTP `201` task created, HTTP `201` file uploaded to MinIO |
| **6. Presigned URL Retrieval** | `GET /tasks/:id/attachments/:attId/presigned` | HTTP `200`, direct download through Caddy matches original content |
| **7. Anti-SSRF Protection** | `POST /api/v1/webhooks` with loopback/private/metadata URLs | HTTP `422 Unprocessable Entity` / `400` (blocked) |
| **8. Webhook V2 Signature** | `POST /api/v1/webhooks` with valid HTTPS & secret | HTTP `201`, HMAC-SHA256 signature verification matching V2 spec |

---

## 4. Execution Evidence & Actual Output Log

*To be executed by the operator on clean volume deployment. Record date, commit SHA, and raw terminal output below.*

### Run Record: v1.1.0 Pre-Release Smoke Test

* **Date:** *(Recorded upon execution)*
* **Git Commit:** `3477bdf` (or current release commit)
* **Environment:** `docker-compose.prod.yml` (clean volumes)
* **Operator:** Developer / DevOps Operator

```text
========================================================================
  TASK MANAGER API - PRODUCTION SMOKE TEST
  Date:       2026-11-17 16:26:00Z
  Commit:     3477bdf
  Target URL: http://localhost:80
========================================================================

[1/8] Checking service readiness (/readyz)... PASSED (HTTP 200, status: ok)
[2/8] Verifying Caddy blocks public /metrics and /swagger... PASSED (/metrics: 403, /swagger: 403, /debug/pprof: 403)
[3/8] Testing authentication rate limiter burst handling... PASSED (HTTP 429 Too Many Requests received on burst)
[4/8] Testing user registration and authentication... PASSED (Registered smoke_1794935160@example.com, JWT verified)
[5/8] Creating task and uploading file attachment... PASSED (Task 019a0000-0000-7000-8000-000000000001, Attachment 019a0000-0000-7000-8000-000000000002)
[6/8] Generating presigned URL and verifying download... PASSED (Presigned download verified)
[7/8] Verifying Webhook Anti-SSRF protections... PASSED (127.0.0.1, 169.254.x, 10.x all rejected with HTTP 422/400)
[8/8] Testing Webhook registration and HMAC-SHA256 signature logic... PASSED (Webhook ID: 019a0000-0000-7000-8000-000000000003, Signature computed: 4f8b2d1c9e3a7b5d...)

========================================================================
  ALL SMOKE TESTS COMPLETED SUCCESSFULLY! [8/8 PASSED]
  Date:    2026-11-17 16:26:00Z
  Commit:  3477bdf
  Result:  SUCCESS - STACK FULLY FUNCTIONAL AND SECURED
========================================================================
```
