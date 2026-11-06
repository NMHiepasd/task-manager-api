-- name: CreateProject :one
INSERT INTO projects (
    owner_id,
    name,
    description,
    color
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListProjectsByOwner :many
SELECT * FROM projects
WHERE owner_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListProjectsKeyset :many
SELECT * FROM projects
WHERE owner_id = $1 AND deleted_at IS NULL
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: CountProjectsByOwner :one
SELECT count(*) FROM projects
WHERE owner_id = $1 AND deleted_at IS NULL;

-- name: UpdateProject :one
UPDATE projects
SET
    name = $2,
    description = $3,
    color = $4,
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND owner_id = $5 AND version = $6 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProject :exec
UPDATE projects
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL;
