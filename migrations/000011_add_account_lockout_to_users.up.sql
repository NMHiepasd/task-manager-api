-- 000011_add_account_lockout_to_users.up.sql

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_users_locked_until
    ON users(locked_until)
    WHERE locked_until IS NOT NULL;

-- Update role check constraint to include 'auditor' role
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin', 'auditor'));
