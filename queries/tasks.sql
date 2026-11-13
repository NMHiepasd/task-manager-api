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
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR due_date <= sqlc.narg('due_before'))
  AND (sqlc.narg('search')::text IS NULL OR to_tsvector('english', title || ' ' || coalesce(description, '')) @@ plainto_tsquery('english', sqlc.narg('search')) OR title ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
      SELECT 1 FROM task_tags tt WHERE tt.task_id = tasks.id AND tt.tag_id = sqlc.narg('tag_id')
  ))
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListTasksKeyset :many
SELECT * FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::varchar IS NULL OR priority = sqlc.narg('priority'))
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR due_date <= sqlc.narg('due_before'))
  AND (sqlc.narg('search')::text IS NULL OR to_tsvector('english', title || ' ' || coalesce(description, '')) @@ plainto_tsquery('english', sqlc.narg('search')) OR title ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
      SELECT 1 FROM task_tags tt WHERE tt.task_id = tasks.id AND tt.tag_id = sqlc.narg('tag_id')
  ))
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: CountTasks :one
SELECT count(*) FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::varchar IS NULL OR priority = sqlc.narg('priority'))
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR due_date <= sqlc.narg('due_before'))
  AND (sqlc.narg('search')::text IS NULL OR to_tsvector('english', title || ' ' || coalesce(description, '')) @@ plainto_tsquery('english', sqlc.narg('search')) OR title ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
      SELECT 1 FROM task_tags tt WHERE tt.task_id = tasks.id AND tt.tag_id = sqlc.narg('tag_id')
  ));

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

-- name: ListTasksDueSoon :many
SELECT 
    t.id,
    t.title,
    t.description,
    t.status,
    t.priority,
    t.due_date,
    t.creator_id,
    t.assignee_id,
    u.email AS recipient_email,
    u.full_name AS recipient_name
FROM tasks t
JOIN users u ON u.id = COALESCE(t.assignee_id, t.creator_id)
WHERE t.deleted_at IS NULL
  AND t.status IN ('todo', 'in_progress')
  AND t.due_date IS NOT NULL
  AND t.due_date > CURRENT_TIMESTAMP
  AND t.due_date <= $1
ORDER BY t.due_date ASC
LIMIT $2;

