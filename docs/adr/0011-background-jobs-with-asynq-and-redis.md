# ADR-011: Background Jobs and Asynchronous Task Processing with Asynq and Redis

## Status
Accepted

## Context
A production REST API performs several operations that should not block the synchronous HTTP request-response cycle or that must execute on recurring schedules:
1. **Periodic Maintenance**: Expired refresh tokens accumulate in the `refresh_tokens` database table over time and must be pruned regularly to maintain indexing performance and table compactness.
2. **Time-Sensitive Reminders**: Tasks approaching their due date (within 24 hours) require timely deadline reminders dispatched to task assignees or creators.
3. **Resilience & Backoff**: Heavy or external tasks (such as sending notification emails or webhooks) must support automatic retries with exponential backoff, priority queue distribution (critical, default, low), and dead-letter handling without crashing the API or losing messages.
4. **Architecture Fit**: Redis 7 is already part of the infrastructure stack for caching and distributed rate limiting, making a Redis-backed distributed task queue the most operational and cost-effective choice compared to creating additional DB contention.

## Decision

### 1. Adopt Asynq (`github.com/hibiken/asynq`)
We adopt `hibiken/asynq` as our distributed asynchronous task processing framework backed by Redis:
- Multi-priority queues:
  - `critical` (weight 6): Used for time-sensitive notifications (e.g. deadline reminders).
  - `default` (weight 3): Used for standard background jobs (e.g. scanning due tasks).
  - `low` (weight 1): Used for maintenance jobs (e.g. deleting expired refresh tokens).
- Dynamic worker concurrency configured via `WORKER_CONCURRENCY` (default: 10 workers).

### 2. Interface Segregation and Decoupled Package Design (`internal/worker`)
Following Clean Architecture and Interface Segregation principles:
- **`TaskDistributor`** interface (`internal/worker/distributor.go`): Abstracts task enqueuing (`client.EnqueueContext`), allowing handlers/services to enqueue background jobs without tight coupling to Asynq internals.
- **`TaskProcessor`** interface (`internal/worker/processor.go`): Defines task handlers. It depends strictly on `TaskStore`, an interface requiring only the exact database queries needed by the background workers (`DeleteExpiredRefreshTokens` and `ListTasksDueSoon`).
- **`Scheduler`** (`internal/worker/scheduler.go`): Manages recurring cron jobs using `asynq.Scheduler`:
  - Expired tokens cleanup: configured by `CLEANUP_CRON_SCHEDULE` (default: `@every 6h`).
  - Deadline scanner: configured by `DEADLINE_REMINDER_CRON_SCHEDULE` (default: `@hourly`).

### 3. Fan-Out Pattern for Deadline Reminders
To decouple periodic scanning from notification delivery:
1. The recurring scheduler enqueues a `task:scan_due_tasks` job every hour into the `default` queue.
2. The scanner worker queries tasks due within the next 24 hours (`ListTasksDueSoon`) with recipient details (`COALESCE(assignee_id, creator_id)`).
3. For each upcoming task, an individual `task:deadline_reminder` task is enqueued into the `critical` queue with up to 5 retries.
4. If invalid payloads occur, tasks fail immediately with `asynq.SkipRetry` to prevent poison-pill queue clogging.

### 4. Lifecycle & Graceful Shutdown
Worker and scheduler lifecycles are integrated into `cmd/api/main.go`:
- Launched in the background when `WORKER_ENABLED=true`.
- On receiving termination signals (`SIGINT`, `SIGTERM`), `processor.Shutdown()`, `scheduler.Shutdown()`, and `distributor.Close()` wait for active tasks to drain safely before exiting.

## Consequences

### Positive
- **Zero Request Thread Blocking**: Heavy or flaky tasks are offloaded from HTTP latency budgets.
- **High Throughput & Low Latency**: Redis in-memory queueing delivers sub-millisecond task enqueueing.
- **Reliable Retries**: Network blips or temporary downstream provider failures are retried automatically with exponential backoff.
- **Testability**: Interfaces (`TaskDistributor`, `TaskStore`) enable 100% unit test coverage with zero external dependencies during unit test suites.

### Negative / Trade-offs
- Requires Redis connectivity; if Redis becomes unreachable, task enqueuing and processing are paused until Redis recovers.
