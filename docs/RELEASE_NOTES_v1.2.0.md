# Release Notes: v1.2.0 — Enterprise Reliability & Security Hardening

**Release Date:** November 27, 2026  
**Git Tag:** `v1.2.0`  
**Previous Release:** `v1.1.1`

---

## Executive Summary

Task Manager API **v1.2.0** delivers major enterprise-grade architectural improvements focusing on **Reliability, Cryptographic Security, and Identity Hardening**:

1. **Transactional Outbox Pattern ([ADR-0020](docs/adr/0020-transactional-outbox-pattern.md))**: Completely eliminates the distributed *dual-write problem* and phantom events. All webhook domain events are atomically committed inside the same PostgreSQL transaction as business operations, then asynchronously delivered to Asynq using a concurrent `SELECT ... FOR UPDATE SKIP LOCKED` relay worker.
2. **AES-256-GCM Webhook Secret Encryption & Decoupled Payloads**: Webhook secrets are encrypted at rest using authenticated symmetric encryption (`pkg/crypto`) and loaded strictly at execution time by background workers, eliminating sensitive credentials from Redis task queues.
3. **Account Lockout Policy ([ADR-0021](docs/adr/0021-account-lockout-and-advanced-rbac.md))**: Protects user accounts from distributed credential-stuffing and botnet brute-force attacks by locking accounts for 15 minutes after 5 consecutive failed login attempts (HTTP 423 `Locked`), with self-healing auto-recovery and dummy hash execution against timing attacks.
4. **Advanced RBAC & Auditor Role ([ADR-0021](docs/adr/0021-account-lockout-and-advanced-rbac.md))**: Introduces the `auditor` role (`domain.RoleAuditor`) enforcing the Principle of Least Privilege and Separation of Duties. Auditors possess read-only visibility into system metrics, audit logs, and delivery lifecycles while being strictly prevented from modifying data or inspecting private user tasks.

---

## What's Changed in Detail

### 1. Transactional Outbox Pattern
- **Atomic Event Generation**: Services (`TaskService`, `CommentService`, `AttachmentService`, `ProjectService`) record events to `outbox_events` in the same database transaction.
- **Relay Worker**: Background loop polls batches using `FOR UPDATE SKIP LOCKED`, preventing race conditions across horizontal worker replicas.
- **Dead-Letter & Failure Recovery**: Failed deliveries increment `retry_count` and move to `status = 'failed'` after 5 attempts.
- **Storage Hygiene**: Processed events older than 7 days are automatically pruned via a recurring Asynq task (`@every 6h`).

### 2. Cryptographic Hardening
- **Standardized AES-256-GCM**: Standalone module `pkg/crypto/aes_gcm.go` creates random 12-byte nonces per encryption call.
- **Payload Decoupling**: Removed secrets from Redis payloads; secrets are fetched and decrypted inside the worker.
- **Zero-Downtime Migration**: Decryption automatically falls back to plaintext if unencrypted legacy secrets are encountered during rolling update.

### 3. Authentication & Account Lockout
- **Lockout Mechanism**: 5 consecutive failed passwords $\to$ 15-minute lock.
- **RFC 9457 HTTP 423**: Standardized Problem Details response (`ACCOUNT_LOCKED`).
- **Timing Attack Mitigation**: Dummy password comparisons ensure constant execution time regardless of account existence or lock status.
- **Auto-Recovery**: No manual admin intervention needed after the 15-minute window expires.

### 4. Advanced RBAC
- **System Roles**: `admin`, `auditor`, `user`.
- **Capability Scopes**: Granular permissions mapped in `internal/domain/role.go`.
- **RBAC Middlewares**: `RequireAnyRole` and `RequirePermission` guarding endpoints.
- **Auditor Restrictions**: Auditor can access `GET /api/v1/system/metrics` and webhook deliveries, but receives HTTP 403 on all business modification endpoints.

---

## Database Migrations

- **`000010_create_outbox_events`**: Creates `outbox_events` table with partial index for pending events.
- **`000011_add_account_lockout_to_users`**: Adds `failed_login_attempts`, `locked_until`, and `last_failed_login_at` to `users`, and expands role check constraint.

To apply migrations:
```bash
make migrate-up
```

---

## Upgrade Guide & Breaking Changes

- **Zero Breaking Changes for API Consumers**: All existing endpoints maintain backward compatibility.
- **Environment Configuration**: Set `WEBHOOK_ENCRYPTION_KEY` (32-byte hex/base64 string) in `.env` or container environment for AES-256-GCM secret encryption.
- **Error Handling**: Clients should handle HTTP `423 Locked` on `/api/v1/auth/login` when account lockout triggers.

---

## Verification & Test Coverage

- **Unit Tests**: 100% pass across all packages with `-race` detector enabled.
- **Integration Tests**: Outbox lifecycle and account lockout queries validated against PostgreSQL 16 Testcontainers.
- **Linter & Security**: `golangci-lint` passes with **0 issues**.
