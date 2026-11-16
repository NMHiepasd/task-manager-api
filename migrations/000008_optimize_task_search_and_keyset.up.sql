-- 000008_optimize_task_search_and_keyset.up.sql

-- 1. Enable pg_trgm for high-performance trigram text indexing
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 2. GIN Trigram index on task titles for fast substring searches
CREATE INDEX IF NOT EXISTS idx_tasks_title_trgm ON tasks 
USING gin (title gin_trgm_ops) 
WHERE deleted_at IS NULL;

-- 3. Composite indexes for keyset pagination partitioned by user
CREATE INDEX IF NOT EXISTS idx_tasks_creator_keyset ON tasks(creator_id, created_at DESC, id DESC) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_assignee_keyset ON tasks(assignee_id, created_at DESC, id DESC) 
WHERE deleted_at IS NULL;

-- 4. Drop redundant single-column created_at index (superseded by idx_tasks_keyset)
DROP INDEX IF EXISTS idx_tasks_created_at;
