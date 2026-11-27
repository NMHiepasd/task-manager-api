CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT chk_outbox_events_status CHECK (status IN ('pending', 'published', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending 
    ON outbox_events (created_at ASC) 
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_outbox_events_published_cleanup 
    ON outbox_events (processed_at ASC) 
    WHERE status = 'published';
