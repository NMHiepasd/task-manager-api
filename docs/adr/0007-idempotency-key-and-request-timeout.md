# ADR-007: Idempotency-Key Header and Request Timeout Strategy

## Status
Accepted

## Context
In distributed web systems and mobile/frontend environments, transient network failures, timeouts, and user double-clicks frequently trigger request retries.
For mutation operations—particularly `POST /api/v1/tasks`—unhandled retries create duplicate database records. Furthermore, unconstrained requests or long-running database queries can cause connection pool exhaustion and denial of service.

We need:
1. A deterministic mechanism to guarantee mutation idempotency according to IETF draft standards (`Idempotency-Key`).
2. Centralized request and database query timeout enforcement to prevent runaway processes.

## Decision

### 1. IETF Idempotency-Key Mechanism
We implement database-backed idempotency using the `Idempotency-Key` HTTP request header:
- **Storage**: An `idempotency_keys` table in PostgreSQL keyed by `(user_id, key)`:
  - Scoping by `user_id` provides strict multi-tenant isolation and prevents cross-user key collisions or IDOR enumeration.
  - Stores `params_hash` (SHA-256 of request payload), `response_code`, and `response_body`.
- **In-Flight Locking**: Inserting a new key sets `locked_at = NOW()` and `response_code = NULL`. Concurrent requests with the same key fail with `409 Conflict` (`IDEMPOTENCY_IN_PROGRESS`).
- **Tampering Detection**: If the same key is reused with a different request body, the middleware rejects it with `409 Conflict` (`IDEMPOTENCY_KEY_MISMATCH`).
- **Cached Replay**: Once completed, the terminal response (code and body) is cached. Subsequent retries return the stored response immediately with header `Idempotent-Replayed: true` without invoking service/database handlers.

### 2. Request & Database Statement Timeout
We establish timeout boundaries at both the HTTP middleware and database driver layers:
- **Middleware Timeout**: `middleware.Timeout(cfg.RequestTimeout)` wraps `c.Request.Context()` with `context.WithTimeout(30s)`. If the context expires, downstream database queries are cancelled, and `middleware.ErrorHandler` returns RFC 9457 `504 Gateway Timeout`.
- **Database Statement Timeout**: `statement_timeout` is configured on the `pgxpool.Pool` connection parameters (`10s`), guaranteeing that runaway SQL queries terminate on the database server even if context cancellation fails.

## Consequences

### Positive
- **Safety Against Duplicates**: Clients can safely retry task creation over unreliable mobile/cellular networks without creating duplicate tasks.
- **Resource Protection**: Bounded execution times prevent connection pool starvation from slow queries.
- **Standards Compliance**: Follows the emerging IETF HTTP API Idempotency-Key specification.

### Negative / Trade-offs
- An additional database write/read occurs for requests providing `Idempotency-Key`.
- Cached idempotency keys must be periodically cleaned up (scheduled deletion target in Phase 10).
