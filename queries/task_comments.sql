-- name: CreateTaskComment :one
INSERT INTO task_comments (
    task_id,
    user_id,
    content
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetTaskCommentByID :one
SELECT tc.id, tc.task_id, tc.user_id, tc.content, tc.created_at, tc.updated_at,
       u.email, u.full_name
FROM task_comments tc
JOIN users u ON u.id = tc.user_id
WHERE tc.id = $1 AND u.deleted_at IS NULL
LIMIT 1;

-- name: ListTaskComments :many
SELECT tc.id, tc.task_id, tc.user_id, tc.content, tc.created_at, tc.updated_at,
       u.email, u.full_name
FROM task_comments tc
JOIN users u ON u.id = tc.user_id
WHERE tc.task_id = $1 AND u.deleted_at IS NULL
ORDER BY tc.created_at ASC, tc.id ASC
LIMIT $2 OFFSET $3;

-- name: ListTaskCommentsKeyset :many
SELECT tc.id, tc.task_id, tc.user_id, tc.content, tc.created_at, tc.updated_at,
       u.email, u.full_name
FROM task_comments tc
JOIN users u ON u.id = tc.user_id
WHERE tc.task_id = $1 AND u.deleted_at IS NULL
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL OR (tc.created_at, tc.id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY tc.created_at ASC, tc.id ASC
LIMIT $2;

-- name: CountTaskComments :one
SELECT count(*) FROM task_comments tc
JOIN users u ON u.id = tc.user_id
WHERE tc.task_id = $1 AND u.deleted_at IS NULL;

-- name: UpdateTaskComment :one
UPDATE task_comments
SET content = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND user_id = $3
RETURNING *;

-- name: DeleteTaskComment :exec
DELETE FROM task_comments
WHERE id = $1 AND task_id = $2;
