# Release Notes - Task Manager REST API v1.1.0

🎉 We are pleased to announce the official **v1.1.0** release of **Task Manager REST API**!

This release delivers enterprise-grade access control, privacy protection, replay-resistant Webhooks V2, and extensive production hardening across all layers.

---

## 🚀 Key Highlights & New Capabilities

1. **Centralized Enterprise Authorization Engine (`internal/authz` & ADR-0018)**:
   - Pure functional authorization policy engine decoupling permission decisions from database and transport layers.
   - Comprehensive role matrix: `owner`, `editor`, `viewer`, and direct creator privileges.
   - Project membership strictly supersedes individual task creator/assignee status for collaborative tasks.
   - Automatic assignee cleanup: removing a member resets their task assignments (`assignee_id = NULL`) to prevent orphan tasks.
   - Multi-tenant Anti-IDOR enforcement: unauthorized resource lookups and mutations consistently return RFC 9457 `404 Not Found` (never 403).

2. **Webhook V2 Architecture (Anti-Replay & Standard Signatures)**:
   - Upgraded to Webhook V2 delivery standard with `X-Webhook-Id`, `X-Webhook-Timestamp`, and `X-Webhook-Signature: v1=<hmac_sha256>`.
   - Constant-time verification library (`pkg/webhook.VerifyV2`) with 5-minute timestamp tolerance window.
   - Strict secret policy: minimum 32 characters or cryptographically generated 24-byte random secrets.
   - Write-only secret visibility: secrets are revealed only once upon creation (`POST /api/v1/webhooks`) and masked in all subsequent read queries.
   - Automatic webhook synchronization: removing or demoting a project member immediately deactivates all their project-scoped webhooks.

3. **User Privacy Protection (PII Sanitization)**:
   - Removed `email` field from `CommentResponse` DTO and `comment.created` webhook event data payloads.
   - Only public collaborative identifiers (`author` display name, `user_id` UUID) are exposed.

4. **Attachment Quotas & Resilient Storage**:
   - Per-task attachment ceilings (`MAX_ATTACHMENTS_PER_TASK`, default: 20) and per-user storage caps (`MAX_USER_STORAGE_MB`, default: 500MB).
   - Safe download headers with `mime.FormatMediaType` filename encoding and `X-Content-Type-Options: nosniff`.
   - Orphan file cleanup worker: failed storage deletions enqueue background retry tasks (`storage:delete_object`) via Asynq.

5. **Production Hardening & Concurrency Safety**:
   - Strict startup configuration validation for `APP_ENV`, `GIN_MODE=release`, `REDIS_PASSWORD`, and `RequestTimeout < InProgressLockTTL`.
   - Race-safe idempotency concurrency control: conditional stale lock deletion using `locked_at` and HTTP 409 `IDEMPOTENCY_IN_PROGRESS` on race collisions.
   - Outbound SSRF defense: custom `safehttp.Client` dialer with DNS re-resolution blocking loopback, private, link-local, CGNAT, and AWS IMDS `169.254.169.254`.
   - Production gating: `/swagger/*` and `/metrics` return HTTP 403 Forbidden in production environments.

6. **Infrastructure Security & CI Pipeline**:
   - Base container images pinned by immutable SHA-256 digests.
   - Automated secret scanning via Gitleaks across entire Git history (119 commits, 0 leaks).
   - Test coverage threshold enforced ($\ge 65\%$ on service layer) and artifact report uploads.
   - Automated production smoke test suite in POSIX Bash (`scripts/smoke-prod.sh`) and PowerShell (`scripts/smoke-prod.ps1`).

---

## ⚠️ Breaking Changes & Migration Notes

- **Task Comments DTO**: The `email` field has been removed from `CommentResponse` and webhook event payloads. Integrations expecting `email` should use `author` or `user_id`.
- **Webhook Signatures**: Legacy `X-Webhook-Signature: sha256=<hex>` has been superseded by Webhook V2 format `X-Webhook-Signature: v1=<hex>`. See [docs/webhooks.md](webhooks.md) for verification examples.

---

## 📋 Docker Quickstart

```bash
# Clone the repository
git clone https://github.com/NMHiepasd/task-manager-api.git
cd task-manager-api

# Check out v1.1.0 release
git checkout v1.1.0

# Configure production environment
cp .env.example .env
# Set strong random values for JWT_SECRET, DB_PASSWORD, REDIS_PASSWORD, and S3_SECRET_KEY

# Launch the production stack
docker compose -f docker-compose.prod.yml up -d --build
```
