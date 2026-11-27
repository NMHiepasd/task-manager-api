# Release Notes - Task Manager REST API v1.1.1

🎉 We are pleased to announce the official **v1.1.1** release of **Task Manager REST API**!

This maintenance and hardening release addresses deployment configuration edge-cases, tightens data consistency across collaborative operations, establishes a comprehensive webhook authorization policy (ADR-0019), and pins the Go toolchain and security scanners to resolve all standard library advisories.

---

## 🚀 Key Highlights & Enhancements

1. **Production Deployment & Gateway Hardening**:
   - **Enforced Production Image Tag**: Required variable substitution `${IMAGE_TAG:?}` in `docker-compose.prod.yml` prevents accidental deployment rollbacks or running unversioned stale images.
   - **MinIO Gateway Reverse Proxy Hardening**: Hardened Caddy reverse proxy routing for object storage (`/task-attachments/*`), explicitly restricting public HTTP requests to `GET` only (blocking unauthorized `PUT`, `DELETE`, `POST`) while ensuring seamless streaming and download.

2. **Transactional Data Consistency & Concurrency Protection**:
   - **Atomic Member Role Management**: Wrapped project member role updates (`UpdateMemberRole`) and member removals (`RemoveMember`) in atomic database transactions (`store.ExecTx`), guaranteeing that member updates/deletions and task unassignments commit together before Redis cache invalidation.
   - **Atomic Attachment Quota Reservation**: Eliminated TOCTOU race conditions under parallel file uploads by implementing PostgreSQL transaction-level advisory locks (`pg_advisory_xact_lock`) with deterministic ordering (`user_id -> task_id`). Added resilient orphan cleanup fallback if subsequent database recording fails.

3. **Webhook Management & Access Policy ([ADR-0019](adr/0019-webhook-management-policy.md))**:
   - **Owner-Exclusive Project Webhook Management**: Project-scoped webhooks are strictly manageable by project owners only (editors and viewers are denied access).
   - **Anti-IDOR Information Masking**: Unauthorized or cross-project webhook lookup and mutation attempts consistently return RFC 9457 `404 Not Found` (never 403).
   - **Inactive Webhook Retention**: Webhooks of removed or demoted members remain disabled and can only be managed, viewed, or deleted by project owners.
   - **Deprecated Legacy Signature Cleanup**: Obsolete legacy signature helper functions `ComputeSignature` and `VerifySignature` have been completely removed from `pkg/webhook`, consolidating exclusively on Webhook V2 (`SignV2`, `VerifyV2`).

4. **Supply Chain Security & Scanner Pinning**:
   - **Go Toolchain Pinning**: Pinned Go toolchain to `go1.27.4` in `go.mod` (`toolchain go1.27.4`) and Dockerfile builder stage (`golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280`).
   - **Zero Vulnerabilities**: Completely eliminated standard library advisories `GO-2026-6859` (`net/http`) and `GO-2026-6809` (`crypto/x509`), achieving **0 vulnerabilities** in `govulncheck ./...`.
   - **Pinned Scanner Tooling**: Pinned exact scanner versions across CI workflows and local documentation: `gosec@v2.30.0`, `gitleaks:v8.24.0`, `trivy:0.76.2`.
   - **CI Go Toolchain Enforcement**: All CI workflows now use `go-version-file: 'go.mod'` with a mandatory `Verify Go Version` step.

5. **Automated Production Smoke Test Suite**:
   - Added automated production smoke test suites in POSIX Bash (`scripts/smoke-prod.sh`) and Windows PowerShell (`scripts/smoke-prod.ps1`).
   - Verifies rate limit isolation, live webhook delivery with Webhook V2 signature verification, and MinIO direct method blocking. Run records documented in [`docs/smoke-test.md`](smoke-test.md).

---

## 📋 Docker Quickstart

```bash
# Clone the repository
git clone https://github.com/NMHiepasd/task-manager-api.git
cd task-manager-api

# Check out v1.1.1 release
git checkout v1.1.1

# Configure production environment
cp .env.example .env
# Set IMAGE_TAG=1.1.1 and supply strong secrets for JWT_SECRET, DB_PASSWORD, REDIS_PASSWORD, S3_SECRET_KEY

# Launch the production stack
docker compose -f docker-compose.prod.yml up -d --build
```
