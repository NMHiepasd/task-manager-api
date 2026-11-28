-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (
    event_type,
    aggregate_type,
    aggregate_id,
    actor_id,
    project_id,
    payload
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: ClaimOutboxEvents :many
UPDATE outbox_events oe
SET
    status = 'processing',
    locked_at = NOW()
WHERE oe.id IN (
    SELECT o.id
    FROM outbox_events o
    WHERE o.status = 'pending' AND o.retry_count < sqlc.arg(max_retries)
    ORDER BY o.created_at ASC
    LIMIT sqlc.arg(batch_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING
    oe.id,
    oe.event_type,
    oe.aggregate_type,
    oe.aggregate_id,
    oe.actor_id,
    oe.project_id,
    oe.payload,
    oe.status,
    oe.retry_count,
    oe.created_at;

-- name: ReclaimStaleOutboxEvents :execrows
UPDATE outbox_events
SET
    status = 'pending',
    locked_at = NULL
WHERE status = 'processing'
  AND locked_at < NOW() - make_interval(secs => sqlc.arg(lease_seconds)::double precision);

-- name: FetchPendingOutboxEvents :many
SELECT 
    id,
    event_type,
    aggregate_type,
    aggregate_id,
    actor_id,
    project_id,
    payload,
    retry_count
FROM outbox_events
WHERE status = 'pending' AND retry_count < $1
ORDER BY created_at ASC
LIMIT $2
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events
SET 
    status = 'published',
    locked_at = NULL,
    processed_at = NOW()
WHERE id = $1 AND status = 'processing';

-- name: MarkOutboxEventsPublished :exec
UPDATE outbox_events
SET 
    status = 'published',
    locked_at = NULL,
    processed_at = NOW()
WHERE id = ANY($1::uuid[]) AND status = 'processing';

-- name: RecordOutboxEventRetry :exec
UPDATE outbox_events
SET 
    status = 'pending',
    locked_at = NULL,
    retry_count = retry_count + 1,
    last_error = $2
WHERE id = $1 AND status = 'processing';

-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events
SET 
    status = 'failed',
    locked_at = NULL,
    last_error = $2,
    processed_at = NOW()
WHERE id = $1 AND status = 'processing';

-- name: DeleteOldProcessedOutboxEvents :execrows
DELETE FROM outbox_events
WHERE status = 'published' AND processed_at < $1;

-- name: GetOutboxEventByID :one
SELECT * FROM outbox_events
WHERE id = $1;

-- name: CountPendingOutboxEvents :one
SELECT COUNT(*) FROM outbox_events
WHERE status = 'pending';

-- name: CountFailedOutboxEvents :one
SELECT COUNT(*) FROM outbox_events
WHERE status = 'failed';
