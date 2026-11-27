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
    processed_at = NOW()
WHERE id = $1;

-- name: MarkOutboxEventsPublished :exec
UPDATE outbox_events
SET 
    status = 'published',
    processed_at = NOW()
WHERE id = ANY($1::uuid[]);

-- name: RecordOutboxEventRetry :exec
UPDATE outbox_events
SET 
    retry_count = retry_count + 1,
    last_error = $2
WHERE id = $1;

-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events
SET 
    status = 'failed',
    last_error = $2,
    processed_at = NOW()
WHERE id = $1;

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
