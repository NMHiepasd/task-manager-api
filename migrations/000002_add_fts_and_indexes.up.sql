-- 000002_add_fts_and_indexes.up.sql

-- 1. Full-Text Search GIN index on Tasks (title and description)
CREATE INDEX IF NOT EXISTS idx_tasks_fts ON tasks 
USING gin(to_tsvector('english', title || ' ' || coalesce(description, ''))) 
WHERE deleted_at IS NULL;

-- 2. Keyset Pagination composite indexes (created_at DESC, id DESC)
CREATE INDEX IF NOT EXISTS idx_tasks_keyset ON tasks(created_at DESC, id DESC) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_projects_keyset ON projects(created_at DESC, id DESC) 
WHERE deleted_at IS NULL;

-- 3. Due Date index for filtering upcoming deadlines
CREATE INDEX IF NOT EXISTS idx_tasks_due_date ON tasks(due_date) 
WHERE deleted_at IS NULL AND due_date IS NOT NULL;
