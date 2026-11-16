-- 000008_optimize_task_search_and_keyset.down.sql

CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC);
DROP INDEX IF EXISTS idx_tasks_assignee_keyset;
DROP INDEX IF EXISTS idx_tasks_creator_keyset;
DROP INDEX IF EXISTS idx_tasks_title_trgm;
