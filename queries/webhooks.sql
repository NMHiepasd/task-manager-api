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
SELECT w.* FROM webhooks w
LEFT JOIN projects p ON p.id = w.project_id AND p.deleted_at IS NULL
LEFT JOIN project_members pm ON pm.project_id = w.project_id AND pm.user_id = w.user_id
WHERE w.is_active = true
  AND ('*' = ANY(w.events) OR sqlc.arg('event_type')::text = ANY(w.events))
  AND (
      -- Case 1: Personal webhook (project_id IS NULL) receives events triggered by the webhook's owner
      (w.project_id IS NULL AND w.user_id = sqlc.arg('user_id'))
      OR
      -- Case 2: Project-scoped webhook receives events matching that project, provided the webhook creator is still an owner or editor
      (
          w.project_id IS NOT NULL
          AND sqlc.narg('project_id')::uuid IS NOT NULL
          AND w.project_id = sqlc.narg('project_id')
          AND p.id IS NOT NULL
          AND (p.owner_id = w.user_id OR pm.role IN ('owner', 'editor'))
      )
  );

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
