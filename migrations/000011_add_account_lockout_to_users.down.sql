-- 000011_add_account_lockout_to_users.down.sql

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));

DROP INDEX IF EXISTS idx_users_locked_until;

ALTER TABLE users
    DROP COLUMN IF EXISTS last_failed_login_at,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_login_attempts;
