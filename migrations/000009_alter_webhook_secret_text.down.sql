-- Rollback Migration 000009: Revert webhook secret column back to VARCHAR(255)
ALTER TABLE webhooks ALTER COLUMN secret TYPE VARCHAR(255);
