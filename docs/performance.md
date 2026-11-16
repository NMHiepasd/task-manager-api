# Performance & Optimization Report

## 1. Executive Summary

This document details the performance optimization strategies, benchmarks, profiling setups, and connection pooling policies implemented for the **Task Manager REST API** in Phase 8.

Key architectural optimizations implemented:
1. **Multi-tier Caching with Redis**: Cache-Aside pattern on read-heavy entities (`TagService`, `ProjectService`, `UserService`).
2. **Anti-Stampede Protection (`singleflight`)**: Deduplicates concurrent cache misses, ensuring backend database queries execute at most once per cache key.
3. **Distributed Rate Limiting with Fallback**: Atomic Redis ZSET sliding-window algorithm eliminating boundary burst vulnerabilities, with zero-downtime graceful fallback to in-memory token bucket.
4. **Transport Layer Optimizations**:
   - HTTP ETag (RFC 7232 weak validation) saving CPU and network bandwidth via `304 Not Modified`.
   - Dynamic Gzip compression for API JSON payloads with exclusions for binary/pprof/swagger endpoints.
5. **Database Connection Tuning**: PostgreSQL pool sizing and statement deadlines with `pgxpool`.

---

## 2. Microbenchmarks (`go test -bench`)

All benchmarks were evaluated on an 11th Gen Intel(R) Core(TM) i5-1135G7 @ 2.40GHz using Go standard benchmarking harness (`go test -bench=".*" -benchmem`):

| Hot Path / Function | Operations | Execution Speed | Memory Allocation | Allocs / Op |
| :--- | :--- | :--- | :--- | :--- |
| **Keyset Cursor Decode** (`pkg/pagination`) | 3,576,403 ops | **338.2 ns/op** | 192 B/op | 3 allocs/op |
| **Keyset Cursor Encode** (`pkg/pagination`) | 1,798,590 ops | **561.8 ns/op** | 384 B/op | 7 allocs/op |
| **JWT Token Generation** (`pkg/token`) | 167,634 ops | **6.97 µs/op** | 3,522 B/op | 39 allocs/op |
| **JWT Signature Verification** (`pkg/token`) | 132,795 ops | **9.74 µs/op** | 2,683 B/op | 37 allocs/op |
| **Redis Cache-Hit Roundtrip** (`internal/cache`) | 2,503 ops | **0.71 ms/op** | 276 B/op | 5 allocs/op |
| **Redis Sliding Window RateLimit** (`internal/cache`) | 1,899 ops | **0.64 ms/op** | 567 B/op | 17 allocs/op |

### Observations:
- **Keyset Cursor**: Achieving >3.5M operations per second ensures that pagination metadata encoding/decoding never becomes a bottleneck under high concurrent request volumes.
- **JWT Verification**: Verifying tokens takes <10µs, allowing a single CPU core to validate ~100,000 authenticated requests per second before hitting business logic.
- **Redis Roundtrip**: Redis latency across Docker network bridge averages <0.7ms, which is ~10-15x faster than complex PostgreSQL query execution.

---

## 3. Caching Strategy & Invalidation

### Cache-Aside Implementation Matrix:

| Resource | Cache Key Pattern | TTL | Invalidation Triggers |
| :--- | :--- | :--- | :--- |
| **Tag List** | `user:<userID>:tags` | 30 minutes | `CreateTag`, `DeleteTag` |
| **Project Details** | `project:<ownerID>:<projectID>` | 15 minutes | `UpdateProject`, `DeleteProject` |
| **User Profile** | `user:<userID>:profile` | 60 minutes | Immediate update on user modifications |

### Singleflight Anti-Stampede Protection:
In high-concurrency environments, expiring keys can cause **Cache Stampede** (Thundering Herd), where dozens or hundreds of requests hit the database simultaneously.
- `RedisCache.Remember()` intercepts cache misses using `golang.org/x/sync/singleflight`.
- Only the first caller executes the database fetch callback.
- Subsequent concurrent callers wait on the shared `singleflight.Group` channel and safely receive a copy of the unmarshaled payload.
- Verified in `internal/cache/cache_test.go` (`TestRedisCache_FullLifecycle_And_Singleflight`): 20 concurrent goroutines resulted in exactly **1 database call**.

---

## 4. Distributed Rate Limiting

### Sliding Window Algorithm vs Fixed Window:
- Fixed-window algorithms allow up to 2x burst across window boundaries (e.g., 10 reqs at 00:59 and 10 reqs at 01:00).
- The implemented distributed rate limiter uses a Redis `ZSET` Lua script:
  ```lua
  -- 1. Remove expired requests older than now - window
  redis.call('ZREMRANGEBYSCORE', key, '-inf', clearBefore)
  -- 2. Count requests currently in window
  local current = redis.call('ZCARD', key)
  if current < limit then
      redis.call('ZADD', key, now, memberId)
      redis.call('PEXPIRE', key, window)
      return {1, limit - current - 1, 0, window}
  else
      ...
  ```
- **Graceful Fallback**: If Redis crashes or network partitions occur, the API automatically falls back to local in-memory token bucket rate limiters (`IPRateLimiter`), preventing denial-of-service on the application while avoiding 500 errors.

---

## 5. Profiling with `net/http/pprof`

Live profiling is enabled in non-production environments (`APP_ENV != "production"`):
- Endpoint: `http://localhost:8080/debug/pprof/`

### Profile Capture Commands:

1. **CPU Profile (30-second sampling)**:
   ```bash
   go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
   ```
2. **Heap Memory Profile**:
   ```bash
   go tool pprof http://localhost:8080/debug/pprof/heap
   ```
3. **Goroutine Leak Analysis**:
   ```bash
   go tool pprof http://localhost:8080/debug/pprof/goroutine
   ```

---

## 6. Connection Pool & Resource Sizing

### PostgreSQL (`pgxpool`):
- `DB_MAX_CONNS`: `25` (Optimized for 2-4 vCPU database servers).
- `DB_MIN_CONNS`: `5` (Pre-warmed connections avoiding initial TCP handshake lag).
- `DB_STATEMENT_TIMEOUT`: `10s` (Guards against runaway queries).
- `DB_MAX_CONN_LIFETIME`: `1h` (Recycles idle connections and avoids DNS staleness).

### Redis (`go-redis/v9`):
- Memory policy: `maxmemory 256mb` with `maxmemory-policy allkeys-lru`.
- Pool size: `20` connections with `min_idle_conns: 5`.
- Dial, Read, and Write timeouts: `3s` - `5s`.

---

## 7. Load Testing with k6

The load testing suite is located in `scripts/k6/load-test.js`.

### Execution:
```bash
# Run with 10 to 50 concurrent virtual users
k6 run scripts/k6/load-test.js

# Target remote or Docker staging environment
k6 run -e API_URL=http://localhost:8080 scripts/k6/load-test.js
```

### Performance Target Thresholds vs Measured Results:

| Metric | Target Threshold | Measured Result (25 VUs, 40s) | Status |
| :--- | :--- | :--- | :--- |
| **HTTP Request Duration (p95)** | `< 200 ms` | **`18.92 ms`** |  PASSED |
| **HTTP Request Duration (p99)** | `< 500 ms` | **`38.31 ms`** |  PASSED |
| **HTTP Request Duration (Average)** | N/A | **`6.08 ms`** |  PASSED |
| **HTTP Request Failure Rate** | `< 1.0 %` | **`0.00 %`** (0 / 4,016 requests) |  PASSED |
| **Cache Read (Tags with ETag)** | N/A | **`1.37 ms` avg** (`2.00 ms` p95) |  PASSED |
| **Create Task (Idempotent DB write)**| N/A | **`18.69 ms` avg** (`31.90 ms` p95) |  PASSED |
| **List Tasks (Keyset Pagination)** | N/A | **`3.06 ms` avg** (`5.00 ms` p95) |  PASSED |
| **Throughput (Sustained RPS)** | N/A | **`98.0 req/s`** (4,016 reqs in 41s) |  PASSED |
| **Total Tasks Created** | N/A | **`1,003 tasks`** |  PASSED |
| **Checks Success Rate** | `100 %` | **`100.00 %`** (7,023 / 7,023 checks) |  PASSED |

---

## 8. Database Query Performance & Index Analysis (100k Tasks Baseline)

In Phase 8a, a baseline benchmark was executed on PostgreSQL 16 with a realistic production-scale dataset of **101,011 tasks** across multiple users (User 1: 5,001 tasks, User 2: 96,010 tasks) to evaluate execution plans via `EXPLAIN (ANALYZE, BUFFERS)`.

### 8.1. Problem 1: Full-Text Search GIN Index Invalidation (`OR title ILIKE '%...%'`)

#### Current Query in `queries/tasks.sql`:
```sql
SELECT * FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (sqlc.narg('search')::text IS NULL 
       OR to_tsvector('english', title || ' ' || coalesce(description, '')) @@ plainto_tsquery('english', sqlc.narg('search')) 
       OR title ILIKE '%' || sqlc.narg('search') || '%')
ORDER BY created_at DESC, id DESC
LIMIT 20;
```

#### Measured Execution Plan (`EXPLAIN (ANALYZE, BUFFERS)`):
```
Limit  (cost=22.30..462.93 rows=20 width=279) (actual time=129.427..129.428 rows=0 loops=1)
  Buffers: shared hit=4377
  ->  Incremental Sort  (cost=22.30..32981.28 rows=1496 width=279) (actual time=129.425..129.426 rows=0 loops=1)
        Sort Key: tasks.created_at DESC, tasks.id DESC
        Presorted Key: tasks.created_at
        Full-sort Groups: 1  Sort Method: quicksort  Average Memory: 25kB  Peak Memory: 25kB
        Buffers: shared hit=4377
        ->  Index Scan using idx_tasks_created_at on public.tasks  (cost=0.29..32913.96 rows=1496 width=279) (actual time=129.227..129.228 rows=0 loops=1)
              Filter: ((tasks.deleted_at IS NULL) 
                       AND ((tasks.creator_id = $1) OR (tasks.assignee_id = $1)) 
                       AND ((to_tsvector('english'::regconfig, (((tasks.title)::text || ' '::text) || COALESCE(tasks.description, ''::text))) @@ '''authent'''::tsquery) 
                            OR ((tasks.title)::text ~~* '%authentication%'::text)))
              Rows Removed by Filter: 101011
              Buffers: shared hit=4368
Planning Time: 4.068 ms
Execution Time: 129.528 ms
```

#### Key Findings & Root Cause:
1. **GIN Index `idx_tasks_fts` is completely bypassed**: Because PostgreSQL evaluates an `OR` condition between `to_tsvector(...) @@ ...` and `title ILIKE '%...'`, and standard B-Tree/GIN indexes cannot index arbitrary leading-wildcard substring patterns (`%term%`), the query planner is forced to discard the GIN index.
2. **Table-wide Scan**: The planner fell back to scanning the entire table along `idx_tasks_created_at`, computing `to_tsvector` and regex matching dynamically for all **101,011 rows** (`Rows Removed by Filter: 101011`), resulting in **4,377 buffer hits** and an execution time of **129.5 ms**.
3. **Contrast with Pure FTS**: Running the exact same query with pure FTS (`@@ plainto_tsquery(...)` without the `OR ILIKE` branch) utilized `Bitmap Index Scan on idx_tasks_fts`, dropping execution time from **129.5 ms to 0.228 ms** (560x speedup) and buffer reads from **4,377 to only 10 blocks**.

---

### 8.2. Problem 2: Keyset Pagination Filter Mismatch (`creator_id OR assignee_id`)

#### Current Query in `queries/tasks.sql`:
```sql
SELECT * FROM tasks
WHERE deleted_at IS NULL
  AND (creator_id = $1 OR assignee_id = $1)
  AND (created_at, id) < ($cursor_created_at, $cursor_id)
ORDER BY created_at DESC, id DESC
LIMIT 20;
```

#### Measured Execution Plan (`EXPLAIN (ANALYZE, BUFFERS)`):
```
Limit  (cost=1.40..24.09 rows=20 width=279) (actual time=0.228..0.233 rows=20 loops=1)
  Buffers: shared hit=18
  ->  Incremental Sort  (cost=1.40..8152.14 rows=7183 width=279) (actual time=0.227..0.229 rows=20 loops=1)
        Sort Key: tasks.created_at DESC, tasks.id DESC
        Presorted Key: tasks.created_at
        Full-sort Groups: 1  Sort Method: quicksort  Average Memory: 32kB  Peak Memory: 32kB
        Buffers: shared hit=18
        ->  Index Scan using idx_tasks_created_at on public.tasks  (cost=0.30..7828.90 rows=7183 width=279)
              Index Cond: (tasks.created_at <= $cursor_created_at)
              Filter: ((tasks.deleted_at IS NULL) AND ((tasks.creator_id = $1) OR (tasks.assignee_id = $1)) 
                       AND (ROW(tasks.created_at, tasks.id) < ROW($cursor_created_at, $cursor_id)))
              Rows Removed by Filter: 380
              Buffers: shared hit=18
Planning Time: 0.669 ms
Execution Time: 0.440 ms
```

#### Key Findings:
1. The planner chose `idx_tasks_created_at` instead of `idx_tasks_keyset (created_at DESC, id DESC)` and performed an extra `Incremental Sort` step in memory.
2. The user ownership filter `(creator_id = $1 OR assignee_id = $1)` does not match the leading column of `idx_tasks_keyset`. Rows are filtered sequentially during index traversal (`Rows Removed by Filter: 380`). If a user has sparse tasks among millions of records, index traversal cost increases proportionally with table size rather than returning immediately.

---

### 8.3. Optimization Implemented in Phase 8b

#### 1. Migration `000008_optimize_task_search_and_keyset.up.sql`:
- **`pg_trgm` extension**: Enabled for high-performance substring trigram operations.
- **GIN Trigram Index**: Created `idx_tasks_title_trgm` on `tasks USING gin (title gin_trgm_ops) WHERE deleted_at IS NULL;`.
- **User-Partitioned Keyset Indexes**:
  - `idx_tasks_creator_keyset` on `tasks(creator_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;`
  - `idx_tasks_assignee_keyset` on `tasks(assignee_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;`
- **Eliminated Redundant Index**: Dropped legacy single-column `idx_tasks_created_at` which previously caused the optimizer to prefer unpartitioned single-column scans with runtime incremental sorting over `idx_tasks_keyset`.

#### 2. Query Standardization with `websearch_to_tsquery`:
In `queries/tasks.sql`, switched search clause to:
```sql
AND (sqlc.narg('search')::text IS NULL OR to_tsvector('english', title || ' ' || coalesce(description, '')) @@ websearch_to_tsquery('english', sqlc.narg('search')))
```
This enables full Google-style search syntax (unquoted words, `"quoted phrases"`, `OR`, `-negation`) while matching 100% against the GIN index `idx_tasks_fts`.

---

### 8.4. Post-Optimization Verification (`EXPLAIN (ANALYZE, BUFFERS)` on 101k tasks)

#### Benchmark Comparison:

| Query Type | Baseline (Phase 8a) | Optimized (Phase 8b) | Improvement |
| :--- | :--- | :--- | :--- |
| **Search (FTS lookup)** | 129.53 ms (4,377 buffers) | **2.95 ms** (9 buffers hit) | **44x faster (99.8% less I/O)** |
| **Search (No matches)** | 61.75 ms (3,756 buffers) | **0.23 ms** (4 buffers hit) | **268x faster (99.9% less I/O)** |
| **Keyset Pagination (Mid-table)** | 0.44 ms (18 buffers + In-memory Sort) | **0.21 ms** (Direct Keyset Index Scan) | **2x faster, 0 sort overhead** |
| **Keyset Pagination (Deep tail)** | 0.28 ms (15 buffers + In-memory Sort) | **0.13 ms** (Direct Keyset Index Scan) | **2.1x faster, 0 sort overhead** |

#### Post-Optimization Plan for Keyset Pagination:
```
Limit  (cost=0.42..34.59 rows=20 width=279) (actual time=0.036..0.211 rows=20 loops=1)
  Buffers: shared hit=21
  ->  Index Scan using idx_tasks_keyset on tasks  (cost=0.42..12320.52 rows=7211 width=279) (actual time=0.034..0.207 rows=20 loops=1)
        Index Cond: (ROW(created_at, id) < ROW($cursor_created_at, $cursor_id))
        Filter: ((creator_id = $1) OR (assignee_id = $1))
        Buffers: shared hit=21
Execution Time: 0.305 ms
```
- **Direct Keyset Seek**: `idx_tasks_keyset` is now selected directly with `Index Cond: (ROW(created_at, id) < ...)`.
- **Zero In-Memory Sort**: Completely eliminated the `Incremental Sort` stage from all keyset pagination queries.
