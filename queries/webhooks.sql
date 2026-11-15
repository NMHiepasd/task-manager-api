-- name: CreateWebhook :one
INSERT INTO webhooks (
    user_id,
    project_id,
    url,
    secret,
    events,
    is_active
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetWebhookByID :one
SELECT * FROM webhooks
WHERE id = $1;

-- name: ListWebhooksByUserID :many
SELECT * FROM webhooks
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListWebhooksByProjectID :many
SELECT * FROM webhooks
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: UpdateWebhook :one
UPDATE webhooks
SET
    url = COALESCE(sqlc.narg('url'), url),
    events = COALESCE(sqlc.narg('events'), events),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks
WHERE id = $1;

-- name: ListActiveWebhooksForEvent :many
SELECT * FROM webhooks
WHERE is_active = true
  AND (user_id = $1 OR (project_id IS NOT NULL AND project_id = $2))
  AND ('*' = ANY(events) OR sqlc.arg('event_type')::text = ANY(events));

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (
    webhook_id,
    event_type,
    payload,
    response_status,
    response_body,
    execution_duration_ms,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetWebhookDeliveryByID :one
SELECT * FROM webhook_deliveries
WHERE id = $1;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1
ORDER BY delivered_at DESC
LIMIT $2 OFFSET $3;
