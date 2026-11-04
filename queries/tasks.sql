-- name: CreateTask :one
INSERT INTO tasks (
    title,
    description,
    status,
    priority,
    due_date
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks
WHERE id = $1 LIMIT 1;

-- name: ListTasks :many
SELECT * FROM tasks
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateTask :one
UPDATE tasks
SET 
    title = COALESCE($2, title),
    description = COALESCE($3, description),
    status = COALESCE($4, status),
    priority = COALESCE($5, priority),
    due_date = COALESCE($6, due_date),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1;
