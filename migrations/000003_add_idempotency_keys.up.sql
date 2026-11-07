-- Create idempotency_keys table for safe mutation retry handling
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_path VARCHAR(255) NOT NULL,
    params_hash VARCHAR(64) NOT NULL,
    response_code INT NULL,
    response_body BYTEA NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_created_at ON idempotency_keys (created_at);
