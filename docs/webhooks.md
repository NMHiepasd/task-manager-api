# Webhook Integration & Signature Verification Guide (Webhook V2)

This document provides instructions for receiving, validating, and handling webhook event deliveries sent by the **Task Manager REST API**.

---

## 1. Overview & Architecture

The Task Manager API delivers real-time notifications for system events (such as task creation, updates, deletions, comments, attachments, and project member changes) asynchronously via Redis-backed [Asynq](https://github.com/hibiken/asynq) background workers.

To guarantee authenticity and prevent replay attacks, all webhook deliveries use **Webhook V2 (Replay-Resistant)** signing based on **HMAC-SHA256**.

---

## 2. HTTP Request Headers

Every webhook POST request delivered to your endpoint contains the following headers:

| Header | Example | Description |
| :--- | :--- | :--- |
| `X-Webhook-Id` | `e4b2d56a-1234-4567-89ab-cdef01234567` | Unique Delivery UUID. **Stable across all retries** of the same event. |
| `X-Webhook-Timestamp` | `1763384400` | Unix epoch timestamp (seconds) when this specific delivery attempt was transmitted. **Updated on every retry**. |
| `X-Webhook-Signature` | `v1=4f6a9b1c...` | Cryptographic HMAC-SHA256 signature prefixed with `v1=`. |
| `X-Webhook-Event` | `task.created` | The subscribed event type that triggered the webhook. |
| `Content-Type` | `application/json` | Request payload format. |
| `User-Agent` | `TaskManager-Webhook/2.0` | Client user agent string. |

---

## 3. Payload Envelope Format

Webhook events wrap their payload in a standardized JSON envelope:

```json
{
  "id": "e4b2d56a-1234-4567-89ab-cdef01234567",
  "event": "task.created",
  "timestamp": "2026-11-17T12:00:00Z",
  "actor_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "data": {
    "task_id": "c1f7a0b2-4d5e-6f7a-8b9c-0d1e2f3a4b5c",
    "project_id": "a0b1c2d3-e4f5-6a7b-8c9d-0e1f2a3b4c5d",
    "title": "Build landing page",
    "status": "todo",
    "priority": "high"
  }
}
```

> [!NOTE]
> The top-level `"id"` field inside the JSON envelope matches the `X-Webhook-Id` header.
>
> **Privacy Hardening**: In compliance with user privacy protections, event payloads (such as `comment.created`) do not expose personal emails. They only include public identifiers: `user_id` (author UUID) and `author` (display name).

---

## 4. Verification Algorithm (Webhook V2)

To verify that a request genuinely originated from Task Manager API and has not been tampered with or replayed:

1. **Extract Headers**:
   - `id` from `X-Webhook-Id`
   - `ts` from `X-Webhook-Timestamp`
   - `signature` from `X-Webhook-Signature` (remove optional `v1=` prefix)

2. **Verify Timestamp Freshness (Replay Attack Prevention)**:
   - Convert `ts` to integer Unix seconds.
   - Compute difference: `delta = |current_time - ts|`.
   - **Recommended tolerance: 5 minutes (300 seconds)**.
   - If `delta > 300s`, reject the request with HTTP 400 or 401. This defends against replay attacks and clock drift.

3. **Construct Signed Payload**:
   - Concatenate `<timestamp>.<delivery_id>.<raw_body_bytes>`.
   - **Important**: Use the **raw, untouched request body bytes**. Do not unmarshal and re-marshal the JSON, as field ordering or whitespace discrepancies will invalidate the signature.

4. **Compute and Compare HMAC**:
   - Compute HMAC-SHA256 digest using your webhook pre-shared secret key over the signed payload bytes.
   - Compare the computed hex digest with the provided signature using **constant-time string/byte comparison** (e.g. `crypto/hmac.Equal` in Go, `crypto.timingSafeEqual` in Node.js, `hmac.compare_digest` in Python) to prevent timing side-channel attacks.

---

## 5. Idempotency & Deduplication

Task Manager API implements **at-least-once delivery**. Transient network timeouts, client disconnects, or 5xx receiver errors trigger automated retries (up to `WEBHOOK_MAX_RETRY` times, default: 6).

Because retries can occur:
- Record processed `X-Webhook-Id` values in an in-memory store (e.g., Redis `SETNX` with a 24-hour TTL) or a relational database table.
- If an incoming request carries an `X-Webhook-Id` that has already been processed, immediately return `HTTP 200 OK` and skip processing the business logic.

---

## 6. Secret Management & Rotation

- **Secret Strength**: When creating a webhook, user-provided secrets must be **at least 32 characters**. If omitted, the server automatically generates a secure 24-byte cryptographically random hex string (`whsec_...`).
- **Write-Only Visibility**: The signing secret is **only returned once** in the `POST /api/v1/webhooks` creation response. It is never returned in `GET`, `LIST`, or `PATCH` endpoints.
- **Rotation Procedure**:
  1. Create a new webhook subscription with a new secret targeting your receiver.
  2. Update your receiver to accept either the old or new secret during migration.
  3. Delete the old webhook subscription via `DELETE /api/v1/webhooks/:id`.

---

## 7. Implementation Examples

### Go

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func verifyWebhook(secret string, r *http.Request, tolerance time.Duration) ([]byte, error) {
	ts := r.Header.Get("X-Webhook-Timestamp")
	id := r.Header.Get("X-Webhook-Id")
	sigHeader := r.Header.Get("X-Webhook-Signature")

	if ts == "" || id == "" || sigHeader == "" {
		return nil, errors.New("missing webhook authentication headers")
	}

	// 1. Check timestamp freshness
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return nil, errors.New("invalid timestamp format")
	}
	tsTime := time.Unix(sec, 0)
	now := time.Now()
	if now.Sub(tsTime) > tolerance || tsTime.Sub(now) > tolerance {
		return nil, errors.New("webhook timestamp expired or too far in future")
	}

	// 2. Read raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// 3. Compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write([]byte(id))
	mac.Write([]byte("."))
	mac.Write(body)
	expectedBytes := mac.Sum(nil)

	// 4. Compare constant-time
	cleanedSig := strings.TrimPrefix(sigHeader, "v1=")
	providedBytes, err := hex.DecodeString(cleanedSig)
	if err != nil || !hmac.Equal(providedBytes, expectedBytes) {
		return nil, errors.New("invalid webhook signature")
	}

	return body, nil
}
```

---

### Node.js (Express / Fastify)

```javascript
const crypto = require('crypto');

function verifyWebhook(secret, rawBody, headers, toleranceSec = 300) {
  const ts = headers['x-webhook-timestamp'];
  const id = headers['x-webhook-id'];
  const sigHeader = headers['x-webhook-signature'];

  if (!ts || !id || !sigHeader) {
    throw new Error('Missing webhook signature headers');
  }

  // 1. Check timestamp tolerance
  const now = Math.floor(Date.now() / 1000);
  const tsInt = parseInt(ts, 10);
  if (isNaN(tsInt) || Math.abs(now - tsInt) > toleranceSec) {
    throw new Error('Webhook timestamp expired or outside tolerance window');
  }

  // 2. Construct signed payload
  const signedPayload = Buffer.concat([
    Buffer.from(`${ts}.${id}.`),
    Buffer.isBuffer(rawBody) ? rawBody : Buffer.from(rawBody, 'utf8')
  ]);

  // 3. Compute expected HMAC
  const expectedSig = crypto
    .createHmac('sha256', secret)
    .update(signedPayload)
    .digest('hex');

  // 4. Constant-time comparison
  const cleanSig = sigHeader.startsWith('v1=') ? sigHeader.slice(3) : sigHeader;
  const expectedBuffer = Buffer.from(expectedSig, 'hex');
  const providedBuffer = Buffer.from(cleanSig, 'hex');

  if (
    expectedBuffer.length !== providedBuffer.length ||
    !crypto.timingSafeEqual(expectedBuffer, providedBuffer)
  ) {
    throw new Error('Invalid webhook signature');
  }

  return true;
}
```

---

### Python (FastAPI / Flask)

```python
import hmac
import hashlib
import time

def verify_webhook(secret: str, raw_body: bytes, headers: dict, tolerance_sec: int = 300) -> bool:
    ts = headers.get("X-Webhook-Timestamp") or headers.get("x-webhook-timestamp")
    delivery_id = headers.get("X-Webhook-Id") or headers.get("x-webhook-id")
    sig_header = headers.get("X-Webhook-Signature") or headers.get("x-webhook-signature")

    if not ts or not delivery_id or not sig_header:
        raise ValueError("Missing webhook signature headers")

    # 1. Verify timestamp freshness
    try:
        ts_int = int(ts)
    except ValueError:
        raise ValueError("Invalid timestamp format")

    now = int(time.time())
    if abs(now - ts_int) > tolerance_sec:
        raise ValueError("Webhook timestamp expired or outside tolerance window")

    # 2. Construct signed payload: <ts>.<id>.<body_bytes>
    prefix = f"{ts}.{delivery_id}.".encode("utf-8")
    signed_payload = prefix + raw_body

    # 3. Compute HMAC-SHA256
    expected_hex = hmac.new(
        key=secret.encode("utf-8"),
        msg=signed_payload,
        digestmod=hashlib.sha256
    ).hexdigest()

    # 4. Compare constant-time
    clean_sig = sig_header[3:] if sig_header.startswith("v1=") else sig_header
    if not hmac.compare_digest(expected_hex, clean_sig):
        raise ValueError("Invalid webhook signature")

    return True
```

---

## 8. Webhook Management & Authorization Policy (ADR-0019)

The Task Manager API enforces strict Role-Based Access Control (RBAC) and Anti-IDOR boundaries on webhook management:

### 8.1 Scoping Model
- **Personal Webhooks (`project_id IS NULL`)**: Bound strictly to the creator's user account. Only the creator may view, update, delete, trigger test pings, or inspect deliveries.
- **Project Webhooks (`project_id IS NOT NULL`)**: Bound to the project security boundary. Governed by project membership and role:
  - **Project Owner**: Full administrative control across all project webhooks (view, update URL/events, toggle active status, delete, test ping, deliveries audit).
  - **Webhook Creator (Active Editor/Owner)**: Full control over the webhooks they created in the project.
  - **Peer Editor (Non-Creator)**: Denied access (returns `HTTP 404 Not Found`) to prevent tampering and disclosure of sensitive URL tokens (e.g. Slack webhook credentials).
  - **Project Viewer / Former Member / Outsider**: Denied access (`HTTP 404 Not Found`).

### 8.2 Departed & Demoted Member Lifecycle
- When a member is removed from a project or demoted to `viewer`, all project webhooks created by that user are **automatically and atomically deactivated (`is_active = false`)** inside the database transaction.
- Webhooks are **not deleted** in order to preserve forensic audit history (`webhook_deliveries`) and prevent disruption of shared company integrations.
- Custody transitions exclusively to the **Project Owner**, who can audit, update credentials, re-activate, or permanently delete the integration.

