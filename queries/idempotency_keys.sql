-- name: GetIdempotencyKey :one
SELECT key, user_id, request_path, params_hash, response_code, response_body, created_at, locked_at
FROM idempotency_keys
WHERE key = $1 AND user_id = $2;

-- name: CreateIdempotencyKey :exec
INSERT INTO idempotency_keys (
    key,
    user_id,
    request_path,
    params_hash,
    locked_at
) VALUES (
    $1, $2, $3, $4, CURRENT_TIMESTAMP
);

-- name: UpdateIdempotencyKeyResponse :exec
UPDATE idempotency_keys
SET response_code = $3,
    response_body = $4,
    locked_at = NULL
WHERE key = $1 AND user_id = $2;

-- name: DeleteOldIdempotencyKeys :exec
DELETE FROM idempotency_keys
WHERE created_at < $1;
