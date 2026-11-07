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

### Performance Target Thresholds:
- **p95 Latency**: `< 200ms`
- **p99 Latency**: `< 500ms`
- **Error Rate**: `< 1.0%`
