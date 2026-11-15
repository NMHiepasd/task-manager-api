# ADR 017: Webhook System with Asynq Background Queue and HMAC-SHA256 Signatures

## Status
Accepted

## Context
Real-time integration with third-party tools (Slack, Discord, CI/CD pipelines, issue trackers, internal bots) requires a push-based event notification mechanism. Delivering HTTP webhook requests synchronously inside API request lifecycles introduces critical drawbacks:
- External network latency directly degrades client API response times.
- Slow or unresponsive webhook receiver servers can exhaust HTTP worker goroutines (slowloris denial of service).
- Lack of retries means transient receiver outages cause permanent event loss.
- Malicious target URLs can be abused for Server-Side Request Forgery (SSRF) against internal services and cloud metadata endpoints.

Key requirements:
1. **Asynchronous Dispatch**:
   - Decouple webhook delivery from HTTP request lifecycles using Redis-backed Asynq background workers.
   - Enqueue delivery jobs in < 1ms without blocking user actions.
2. **Security & SSRF Mitigation**:
   - Validate target URL protocol (only `http` and `https` permitted).
   - DNS resolution checks to block requests to loopback (`127.0.0.1`, `::1`), private subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), and link-local cloud metadata addresses (`169.254.169.254`).
   - Configurable `ALLOW_LOCAL_WEBHOOKS` flag for local development and integration testing.
3. **Payload Authenticity (HMAC-SHA256)**:
   - Every delivery includes an `X-Webhook-Signature: sha256=<hex_digest>` header computed with the webhook's pre-shared secret.
   - Secrets are automatically generated (24 cryptographically random bytes with `whsec_` prefix) or user-supplied.
4. **Reliability & Retry Mechanism**:
   - Asynq automatically retries failed deliveries (HTTP 5xx or network errors) up to 5 times with exponential backoff.
   - Client errors (HTTP 4xx from the receiver) terminate immediately without unnecessary retries.
5. **Auditing & Troubleshooting**:
   - Record delivery attempts in `webhook_deliveries` (status code, duration in ms, truncated response body, status `success` or `failed`).
   - Manual test ping endpoint (`POST /api/v1/webhooks/:id/test`) for instant connectivity verification.

## Decision
1. **Database Schema**:
   - Migration `000007_create_webhooks.up.sql` creates:
     - `webhooks`: Stores subscriptions, user/project scoping, URLs, secrets, subscribed event arrays, and active flags.
     - `webhook_deliveries`: Audit log capturing execution status, duration, response codes, and payloads.
2. **Supported Events**:
   - `task.created`, `task.updated`, `task.deleted`
   - `comment.created`
   - `attachment.uploaded`
   - `project.member_added`
   - `*` (wildcard - all events)
   - `ping` (test ping)
3. **Decoupled Architecture**:
   - Introduced `EventDispatcher` and `EventDispatchable` interfaces in `internal/service`.
   - Core services (`TaskService`, `CommentService`, `AttachmentService`) emit events asynchronously to `WebhookService` without tight coupling.
4. **HTTP Transport**:
   - `POST /api/v1/webhooks`: Register a webhook subscription.
   - `GET /api/v1/webhooks`: List subscriptions (optionally filtered by `project_id`).
   - `GET /api/v1/webhooks/:id`: Get subscription details.
   - `PATCH /api/v1/webhooks/:id`: Update subscription (URL, events, active status).
   - `DELETE /api/v1/webhooks/:id`: Delete subscription.
   - `POST /api/v1/webhooks/:id/test`: Trigger test ping event.
   - `GET /api/v1/webhooks/:id/deliveries`: Audit history of recent delivery attempts.

## Consequences
### Positive
- Zero latency impact on core API operations.
- Strong security posture with automated HMAC signatures and SSRF defense.
- Full visibility with per-webhook delivery audit logs.
- Automatic retry on transient network failures via Asynq and Redis.

### Negative / Trade-offs
- Requires Redis for asynchronous background processing (gracefully falls back if worker is unconfigured).
- Target receiver endpoint must respond within the 10s worker timeout window.
