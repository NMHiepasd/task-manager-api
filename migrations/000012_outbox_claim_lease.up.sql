-- 000012_outbox_claim_lease.up.sql

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ NULL;

ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS chk_outbox_events_status;
ALTER TABLE outbox_events ADD CONSTRAINT chk_outbox_events_status
    CHECK (status IN ('pending', 'processing', 'published', 'failed'));

CREATE INDEX IF NOT EXISTS idx_outbox_events_processing_lease
    ON outbox_events (locked_at ASC)
    WHERE status = 'processing';
