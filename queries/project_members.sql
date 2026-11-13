-- name: AddProjectMember :one
INSERT INTO project_members (
    project_id,
    user_id,
    role
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetProjectMember :one
SELECT * FROM project_members
WHERE project_id = $1 AND user_id = $2
LIMIT 1;

-- name: GetProjectMemberWithUser :one
SELECT pm.id, pm.project_id, pm.user_id, pm.role, pm.created_at, pm.updated_at,
       u.email, u.full_name
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1 AND pm.user_id = $2 AND u.deleted_at IS NULL
LIMIT 1;

-- name: ListProjectMembers :many
SELECT pm.id, pm.project_id, pm.user_id, pm.role, pm.created_at, pm.updated_at,
       u.email, u.full_name
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1 AND u.deleted_at IS NULL
ORDER BY
    CASE pm.role
        WHEN 'owner' THEN 1
        WHEN 'editor' THEN 2
        WHEN 'viewer' THEN 3
        ELSE 4
    END,
    pm.created_at ASC;

-- name: UpdateProjectMemberRole :one
UPDATE project_members
SET role = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteProjectMember :exec
DELETE FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: CountProjectMembers :one
SELECT count(*) FROM project_members
WHERE project_id = $1;
