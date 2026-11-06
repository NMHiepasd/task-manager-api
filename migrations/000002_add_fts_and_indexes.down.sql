-- 000002_add_fts_and_indexes.down.sql

DROP INDEX IF EXISTS idx_tasks_due_date;
DROP INDEX IF EXISTS idx_projects_keyset;
DROP INDEX IF EXISTS idx_tasks_keyset;
DROP INDEX IF EXISTS idx_tasks_fts;
