# ADR-008: Caching Strategy and Performance Optimization

## Status
Accepted

## Context
As user adoption and request concurrency increase, read-heavy endpoints (tag listings, project statistics, user profiles) place repetitive load on the primary PostgreSQL database. Furthermore:
1. In high-concurrency spikes, cache expiration can trigger **Cache Stampede** (Thundering Herd), where dozens of simultaneous requests bypass the cache and flood the database.
2. In-memory rate limiting operates strictly within a single process, making it ineffective when scaling the API horizontally across multiple container replicas.
3. Network bandwidth and client latency can be significantly reduced by standard HTTP caching semantics (`ETag`, `304 Not Modified`) and payload compression (`gzip`).

## Decision

### 1. Redis Service & Cache-Aside Pattern
- We introduce `redis:7-alpine` into the infrastructure with an LRU eviction policy (`maxmemory 256mb`, `allkeys-lru`).
- We implement the **Cache-Aside** pattern in `TagService`, `ProjectService`, and `UserService`:
  - `user:<userID>:tags` (TTL 30m)
  - `project:<ownerID>:<projectID>` (TTL 15m)
  - `user:<userID>:profile` (TTL 60m)
- Invalidation is immediate and deterministic on mutations (`CreateTag`, `DeleteTag`, `UpdateProject`, `DeleteProject`).
- If Redis is unavailable or unconfigured, the application gracefully falls back to `NoopCache` without downtime or runtime panics.

### 2. Singleflight Anti-Stampede Coordination
- We implement `RedisCache.Remember()` using `golang.org/x/sync/singleflight`.
- When a cache miss occurs, concurrent requests for the same key coordinate so that only one worker executes the database fetch callback.
- Subsequent concurrent callers wait on the shared channel and receive a copy of the deserialized payload, eliminating cache stampede.

### 3. Distributed Sliding-Window Rate Limiting
- We transition from local in-memory token bucket to distributed rate limiting using an atomic Redis `ZSET` Lua script.
- The sliding-window algorithm counts requests within exact millisecond boundaries (`ZREMRANGEBYSCORE`, `ZCARD`), completely preventing fixed-window boundary burst attacks.
- **Graceful Fallback**: If Redis becomes unreachable, the rate limiter middleware automatically falls back to an in-memory token bucket limiter (`IPRateLimiter`), ensuring both API security and operational resilience.

### 4. HTTP Transport Optimizations
- **ETag Middleware**: Generates weak ETags (RFC 7232) using CRC32 checksums of response bodies. When clients supply matching `If-None-Match`, the server returns `304 Not Modified` with zero response body.
- **Gzip Compression**: Compresses JSON responses using `gin-contrib/gzip` with explicit exclusions for binary, swagger, and diagnostic endpoints.

### 5. Diagnostics and Profiling
- We mount `net/http/pprof` endpoints under `/debug/pprof/*` for non-production environments to allow real-time CPU, heap, and goroutine profiling.

## Consequences

### Positive
- **Reduced Database Load**: High-frequency reads are served in <1ms directly from Redis memory.
- **Scalability**: Rate limiting coordinates across multiple stateless API instances.
- **Zero-Downtime Resilience**: Automatic fallback to `NoopCache` and in-memory rate limiting ensures the application never hard-fails if Redis goes offline.
- **Bandwidth Reduction**: ETags and Gzip compression minimize egress network usage.

### Negative / Trade-offs
- Introducing Redis adds an external stateful dependency to the deployment topology.
- Developers must maintain cache invalidation logic whenever adding new mutative operations to cached entities.
