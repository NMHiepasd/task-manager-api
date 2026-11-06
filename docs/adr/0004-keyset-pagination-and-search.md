# ADR-004: Keyset Pagination, Full-Text Search, and Anti-IDOR Strategy

## Status
Accepted

## Context
As the Task Manager API grows to accommodate thousands to millions of tasks and projects, two major performance bottlenecks and one security vulnerability arise with traditional REST API approaches:

1. **Slow Offset Pagination at Scale ($O(N)$ vs $O(1)$)**:
   - Traditional `OFFSET / LIMIT` pagination requires PostgreSQL to scan and discard all rows prior to the target offset. For example, `OFFSET 50000 LIMIT 20` scans 50,020 rows, leading to quadratic degradation on deep pages and severe database buffer pool churn.
   - Offset pagination suffers from pagination drift (duplicate or missed records) when new items are inserted or deleted while a client is paging.

2. **Full-Text Search Performance (`ILIKE '%query%'` Table Scans)**:
   - Wildcard text searches with `LIKE` or `ILIKE` cannot utilize standard B-Tree indexes, resulting in full sequential table scans that do not scale.

3. **Insecure Direct Object Reference (IDOR) Information Disclosure**:
   - Returning `403 Forbidden` when an authenticated user attempts to access an existing task or project owned by another user leaks whether the target UUID exists. Attackers can enumerate sequential or leaked UUIDs to map out private resources.

4. **Atomicity for Task Tag Associations**:
   - Assigning a set of tags to a task involves clearing previous relations and inserting new ones. Doing this without a transaction leads to partial writes if network or constraint errors occur.

## Decision

### 1. Keyset (Cursor-Based) Pagination
We adopt **Keyset Pagination** as the primary pagination mechanism for `/tasks` and `/projects`, while maintaining backwards compatibility with offset pagination:
- **Index Support**: Created composite B-Tree indexes on `(created_at DESC, id DESC)`:
  - `idx_tasks_keyset` on `tasks(created_at DESC, id DESC)`
  - `idx_projects_keyset` on `projects(created_at DESC, id DESC)`
- **Comparison Clause**:
  ```sql
  WHERE ($3::timestamptz IS NULL OR (created_at, id) < ($3::timestamptz, $4::uuid))
  ORDER BY created_at DESC, id DESC
  LIMIT $2
  ```
- **Cursor Format**: An opaque, URL-safe base64 string encoding `RFC3339Nano` timestamp and UUID separated by `|`:
  `base64url(2026-11-06T15:00:00.123456789Z|3fa85f64-5717-4562-b3fc-2c963f66afa6)`
- **Client Ergonomics**: Querying with `limit = N` fetches `N + 1` rows to detect `has_more` without extra `COUNT(*)` queries. When `has_more` is true, the response includes `next_cursor` pointing to the last item.

### 2. Full-Text Search (FTS) with GIN Index
We leverage PostgreSQL native full-text search with an English dictionary:
- **GIN Index**:
  ```sql
  CREATE INDEX IF NOT EXISTS idx_tasks_fts ON tasks
  USING GIN (to_tsvector('english', title || ' ' || coalesce(description, '')));
  ```
- **Search Query**:
  `to_tsvector('english', title || ' ' || coalesce(description, '')) @@ plainto_tsquery('english', $search)`
- Query parameter `?q=<term>` filters tasks across both `title` and `description` with stemming and sub-millisecond execution.

### 3. Anti-IDOR Defense-in-Depth (404 over 403)
To prevent resource enumeration attacks:
- When a user requests, updates, or deletes a task or project that does not belong to them (or they lack access permissions), the service returns `404 Not Found` (`domain.ErrTaskNotFound` / `domain.ErrProjectNotFound`) instead of `403 Forbidden`.
- An attacker probing random UUIDs receives the exact same RFC 9457 Problem Details response whether the resource ID exists for another user or does not exist at all.

### 4. Atomic Tag Updates via `WithTx`
We introduced `repository.Store` extending `repository.Querier` with transaction management:
```go
type Store interface {
    Querier
    WithTx(ctx context.Context, fn func(q Querier) error) error
}
```
Batch tag updates (`PUT /tasks/:id/tags`) execute in a single database transaction, clearing stale associations and inserting new ones atomically with automatic rollback on error.

## Consequences

### Positive
- **Stable $O(1)$ Index Seek**: Paging from page 1 to page 10,000 runs in consistent single-digit milliseconds.
- **Zero Pagination Drift**: New items inserted at the top do not shift keyset windows or cause duplicate cards in mobile feeds.
- **Enhanced Privacy**: UUID probing attacks are thwarted by uniform 404 responses.
- **Fast Full-Text Search**: Stemming-aware indexing with GIN indexes avoids sequential scans on large task backlogs.
- **Data Consistency**: Atomic tag batch operations guarantee no dangling or half-written tag associations.

### Negative / Trade-offs
- Random access jumping to an arbitrary page (e.g., jump directly to page 47) is not supported with keyset pagination. Clients must stream or paginate sequentially using `next_cursor` (or use offset pagination for small static administrative views).
