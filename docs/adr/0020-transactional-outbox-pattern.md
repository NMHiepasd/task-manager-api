# ADR 0020: Transactional Outbox Pattern for Reliable Webhook Event Dispatching

## Status
Accepted

## Context

In distributed and event-driven architectures, state changes in the primary database must often trigger asynchronous event notifications to external systems (such as webhook subscribers, audit loggers, or message brokers).

In the current implementation of Task Manager API:
1. Core services (`TaskService`, `CommentService`, `AttachmentService`) execute business operations inside a PostgreSQL transaction (`pgx.Tx`).
2. After the database transaction commits, the service invokes `WebhookService.DispatchEvent(...)`, which queries matching subscriptions and pushes `worker.SendWebhookPayload` jobs into Redis via Asynq.

### The Dual-Write Problem
This design suffers from the classic **Dual-Write Problem**:
- **Post-Commit Failure**: If the database transaction commits successfully, but the application process crashes, the host runs out of memory, or Redis network connectivity drops immediately before or during event enqueueing, the event is permanently lost. The database reflects the change, but webhook subscribers never receive the event.
- **Pre-Commit Inconsistency**: If the application enqueued the event *before* committing the database transaction, a subsequent database rollback (e.g., serialization failure, constraint violation, or sudden shutdown) would result in a "phantom event" delivered to external consumers for an operation that never actually happened.

### Requirements
To achieve enterprise-grade reliability in v1.2.0, the webhook dispatching system must satisfy:
1. **Atomicity (All-or-Nothing)**: Business state mutations and event generation must occur within the same ACID transaction in PostgreSQL.
2. **At-Least-Once Delivery**: Every persisted event must be reliably relayed to the background task queue (Asynq) even across application restarts, Redis outages, or network partitions.
3. **Concurrency & Horizontal Scalability**: Multiple API server replicas and background workers must be able to process events concurrently without race conditions, duplicates, or lock contention.
4. **Dead-Letter Handling & Observability**: Poison pills or repeatedly failing events must not block the processing pipeline; failed events must be diagnosable via error tracking.
5. **Storage Hygiene**: Successfully processed events must not accumulate indefinitely in the primary relational database.

---

## Decision

We adopt the **Transactional Outbox Pattern** with a relational outbox table (`outbox_events`), batch polling using `SELECT ... FOR UPDATE SKIP LOCKED`, and an automated retention cleanup scheduler.

### 1. Database Schema (`outbox_events`)

An outbox table will be introduced via migration `migrations/000010_create_outbox_events.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT chk_outbox_events_status CHECK (status IN ('pending', 'published', 'failed'))
);
```

#### Field Specifications:
- `id`: Globally unique identifier for idempotency tracking.
- `event_type`: Domain event type (e.g., `task.created`, `task.updated`, `task.deleted`, `comment.created`, `attachment.uploaded`, `project.member_added`).
- `aggregate_type`: The root entity category (`task`, `comment`, `attachment`, `project`).
- `aggregate_id`: UUID of the affected entity.
- `actor_id`: User who initiated the action (enforces auditability and tenant isolation).
- `project_id`: Nullable project scope (NULL for personal tasks, non-NULL for shared project entities).
- `payload`: Structured JSON event envelope containing actor ID, timestamp, and entity payload.
- `status`: Lifecycle state:
  - `pending`: Newly inserted, waiting for relay.
  - `published`: Successfully converted into background tasks and enqueued in Asynq.
  - `failed`: Dead-letter status after exceeding maximum retries.
- `retry_count`: Number of failed attempts to publish to the queue.
- `last_error`: Diagnostic message or stack trace of the latest publishing failure.
- `created_at`: Insertion timestamp.
- `processed_at`: Timestamp when the event transitioned to `published` or `failed`.

### 2. Index Optimization

To maintain sub-millisecond polling speeds even under heavy write loads:

```sql
-- Partial index dedicated to pending event polling
CREATE INDEX idx_outbox_events_pending 
    ON outbox_events (created_at ASC) 
    WHERE status = 'pending';

-- Index dedicated to retention cleanup of published events
CREATE INDEX idx_outbox_events_published_cleanup 
    ON outbox_events (processed_at ASC) 
    WHERE status = 'published';
```

**Why Partial Indexes?**
- In steady state, only a handful of rows have `status = 'pending'`. The partial index remains tiny (occupying minimal buffer cache memory and zero write overhead for rows in other statuses).
- Queries filtering on `WHERE status = 'pending'` perform index scans directly on the micro-index without touching table pages until row retrieval.

### 3. Service Layer Integration

Core business services (`TaskService`, `CommentService`, `AttachmentService`) will stage the outbox record inside the active transaction `pgx.Tx` using sqlc-generated query `InsertOutboxEvent`:

```go
// Inside transaction (e.g., CreateTask)
task, err := txQueries.CreateTask(ctx, arg)
if err != nil {
    return nil, err
}

// Stage outbox event in the same transaction
err = txQueries.InsertOutboxEvent(ctx, repository.InsertOutboxEventParams{
    EventType:     "task.created",
    AggregateType: "task",
    AggregateId:   task.ID,
    ActorID:       actorID,
    ProjectID:     task.ProjectID,
    Payload:       eventPayloadBytes,
})
if err != nil {
    return nil, fmt.Errorf("failed to record outbox event: %w", err)
}

// Commit transaction atomically
if err := tx.Commit(ctx); err != nil {
    return nil, err
}
```

If the transaction succeeds, both the entity modification and the outbox event are guaranteed to be durable. If anything fails, both are rolled back cleanly.

### 4. Relay Strategy (`SELECT ... FOR UPDATE SKIP LOCKED`)

A background Outbox Publisher (relay worker) polls pending events using a lock-free queue pattern:

```sql
-- name: FetchPendingOutboxEvents :many
SELECT id, event_type, aggregate_type, aggregate_id, actor_id, project_id, payload, retry_count
FROM outbox_events
WHERE status = 'pending' AND retry_count < $1
ORDER BY created_at ASC
LIMIT $2
FOR UPDATE SKIP LOCKED;
```

#### Why `FOR UPDATE SKIP LOCKED`?
- **Zero Lock Contention**: When multiple worker instances run concurrently, `SKIP LOCKED` instructs PostgreSQL to lock matching rows and skip rows currently locked by another transaction, rather than waiting.
- **Horizontal Scaling**: Multiple publisher nodes can process batches simultaneously without conflicts or deadlocks.
- **Safety**: Locked rows remain visible only to the active transaction; uncommitted work is isolated.

### 5. Publishing Pipeline & Dead-Letter Handling

For each locked row in the batch:
```
                               +----------------------------+
                               |     outbox_events row      |
                               +----------------------------+
                                             |
                                             v
                           Query matching active webhooks in DB
                           (filtered by event, user, project)
                                             |
                               +-------------+-------------+
                               |                           |
                       Webhooks Found               No Webhooks Found
                               |                           |
                               v                           |
                    Enqueue Asynq tasks                    |
                     (SendWebhookPayload)                  |
                               |                           |
                               +-------------+-------------+
                                             |
                                             v
                       Mark status = 'published', processed_at = NOW()
```

#### Failure Handling:
1. If the Asynq/Redis client returns a transient error during queue enqueueing:
   - Increment `retry_count`.
   - Update `last_error` with error details.
   - If `retry_count < MaxRetries` (default: 5): row remains in `status = 'pending'`, available for the next polling cycle with exponential backoff interval.
   - If `retry_count >= MaxRetries`: update `status = 'failed'`, `processed_at = NOW()`.
2. Operators can monitor `COUNT(*) FROM outbox_events WHERE status = 'failed'` using Prometheus metrics (`outbox_events_failed_total`) and trigger manual inspection or replays.

### 6. Automated Retention & Cleanup

To prevent the table from unbounded growth:
- An Asynq periodic task (`task:cleanup_old_outbox_events`) runs every 6 hours.
- Retention window: **7 days** after `processed_at`.
- Query:
  ```sql
  -- name: CleanupOldOutboxEvents :exec
  DELETE FROM outbox_events
  WHERE status = 'published' AND processed_at < $1;
  ```
- Events in `status = 'failed'` are **not** automatically deleted, ensuring audit traces remain available until acknowledged or resolved by administrators.

---

## Consequences

### Positive
- **Guaranteed Consistency**: Completely eliminates the Dual-Write problem. Webhook events can never be dropped due to application crashes or transient network failures during HTTP requests.
- **Zero Latency Impact on User Requests**: API requests only write a row to PostgreSQL within their existing transaction; no external network calls or Redis enqueueing are performed in the user-facing request path.
- **Concurrency-Safe**: `FOR UPDATE SKIP LOCKED` enables seamless horizontal scaling of publishers across multiple pods or containers without distributed locks.
- **Resilient & Observable**: Dead-letter tracking via `status = 'failed'` and `last_error` prevents poison messages from stalling the queue and exposes actionable telemetry.
- **Clean Separation of Concerns**: Decouples domain write operations from delivery mechanisms and third-party webhook subscriptions.

### Negative / Trade-offs
- **PostgreSQL Write Amplification**: Storing an extra row per domain event adds write I/O to PostgreSQL. Mitigated by compact payload serialization, partial indexing, and automated 7-day pruning.
- **Event Relay Latency**: There is a small polling latency between database commit and queue distribution (typically 200ms - 2s depending on polling frequency). This is entirely acceptable for asynchronous webhook notifications.
- **At-Least-Once Delivery**: In rare scenarios (e.g., worker crashes immediately after enqueuing to Asynq but before marking `outbox_events` as `published`), an event could be published twice. Webhook receivers must implement idempotency checks using the `X-Webhook-Id` header (already standardized in ADR 0017 and ADR 0019).
