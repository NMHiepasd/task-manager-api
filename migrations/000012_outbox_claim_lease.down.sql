-- 000012_outbox_claim_lease.down.sql

DROP INDEX IF EXISTS idx_outbox_events_processing_lease;

UPDATE outbox_events
SET status = 'pending', locked_at = NULL
WHERE status = 'processing';

ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS chk_outbox_events_status;
ALTER TABLE outbox_events ADD CONSTRAINT chk_outbox_events_status
    CHECK (status IN ('pending', 'published', 'failed'));

ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS locked_at;
