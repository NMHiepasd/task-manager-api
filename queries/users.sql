-- name: CreateUser :one
INSERT INTO users (
    email,
    password_hash,
    full_name,
    role
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL
LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateUser :one
UPDATE users
SET
    full_name = COALESCE($2, full_name),
    role = COALESCE($3, role),
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND version = $4 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteUser :exec
UPDATE users
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: RecordFailedLogin :one
UPDATE users
SET failed_login_attempts = CASE
        WHEN last_failed_login_at IS NULL
          OR last_failed_login_at < CURRENT_TIMESTAMP - make_interval(secs => sqlc.arg(window_seconds)::double precision)
          OR (locked_until IS NOT NULL AND locked_until <= CURRENT_TIMESTAMP)
        THEN 1
        ELSE failed_login_attempts + 1
    END,
    last_failed_login_at = CURRENT_TIMESTAMP,
    locked_until = CASE
        WHEN (
            CASE
                WHEN last_failed_login_at IS NULL
                  OR last_failed_login_at < CURRENT_TIMESTAMP - make_interval(secs => sqlc.arg(window_seconds)::double precision)
                  OR (locked_until IS NOT NULL AND locked_until <= CURRENT_TIMESTAMP)
                THEN 1
                ELSE failed_login_attempts + 1
            END
        ) >= sqlc.arg(threshold)::int
        THEN CURRENT_TIMESTAMP + make_interval(secs => sqlc.arg(duration_seconds)::double precision)
        ELSE NULL
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING failed_login_attempts, locked_until;

-- name: ResetFailedLogin :exec
UPDATE users
SET failed_login_attempts = 0,
    locked_until = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

