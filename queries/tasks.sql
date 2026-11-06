-- name: CreateTask :one
INSERT INTO tasks (
    project_id,
    creator_id,
    assignee_id,
    title,
    description,
    status,
    priority,
    due_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::varchar IS NULL OR priority = sqlc.narg('priority'))
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountTasks :one
SELECT count(*) FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::varchar IS NULL OR priority = sqlc.narg('priority'));

-- name: ListTasksByCreator :many
SELECT * FROM tasks
WHERE creator_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateTask :one
UPDATE tasks
SET 
    project_id = $2,
    assignee_id = $3,
    title = $4,
    description = $5,
    status = $6,
    priority = $7,
    due_date = $8,
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND version = $9 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTask :exec
UPDATE tasks
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;
