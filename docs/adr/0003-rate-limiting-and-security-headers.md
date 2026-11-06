# ADR-003: Rate Limiting, Security Headers, and CORS Policy

## Status
Accepted

## Context
Exposing an API directly to public networks introduces risks of brute-force credential stuffing on authentication endpoints (`/auth/login`, `/auth/register`), Denial-of-Service (DoS), Cross-Site Scripting (XSS), MIME-type sniffing, Clickjacking, and unauthorized cross-origin data exfiltration.

To harden the API according to OWASP Top 10 API Security guidelines, we need:
1. Per-client rate limiting on sensitive endpoints without external Redis dependencies in initial phases.
2. Standard defense-in-depth security response headers.
3. Explicit, least-privilege Cross-Origin Resource Sharing (CORS).

## Decision
1. **In-Memory Token Bucket Rate Limiting (`golang.org/x/time/rate`)**:
   - Implemented an IP-keyed Token Bucket rate limiter in `internal/transport/http/middleware/rate_limiter.go`.
   - Applied strict rate limits to sensitive auth endpoints (`/auth/login`, `/auth/register`).
   - Integrated lazy eviction of inactive client IPs to prevent memory leaks during long-running operations.
   - When exceeded, returns HTTP `429 Too Many Requests` compliant with RFC 9457 Problem Details, including the `Retry-After` response header.

2. **OWASP Security Headers**:
   - Attached global security headers via `internal/transport/http/middleware/security_headers.go`:
     - `X-Content-Type-Options: nosniff`: Prevents MIME-sniffing.
     - `X-Frame-Options: DENY`: Protects against Clickjacking.
     - `X-XSS-Protection: 0`: Disables vulnerable/obsolete legacy browser XSS filters.
     - `Referrer-Policy: strict-origin-when-cross-origin`: Restricts referrer information leakage.
     - `Content-Security-Policy: default-src 'none'; frame-ancestors 'none';`: Strict API-first CSP.
     - `Permissions-Policy: geolocation=(), microphone=(), camera=()`: Blocks unused client capabilities.
     - `Strict-Transport-Security: max-age=31536000; includeSubDomains`: Enforced in production environments.

3. **Restricted CORS Policy**:
   - Implemented configurable allowed origins via `CORSAllowedOrigins` configuration.
   - Preflight `OPTIONS` requests are terminated early with `204 No Content`.
   - Allowed methods: `GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD`.
   - Allowed headers: `Origin, Content-Type, Accept, Authorization, X-Request-ID`.
   - Exposed headers: `Content-Length, X-Request-ID, Retry-After`.

## Consequences
- **Positive**:
  - Significant protection against automated credential stuffing and resource exhaustion.
  - Zero external infrastructure dependency (Redis) needed for initial deployment.
  - Full compliance with RFC 9457 error contracts even during rate limiting.
  - Browser clients have clear, secure CORS boundary with credentials support.
- **Trade-offs**:
  - In-memory rate limiting applies per-replica. When scaled horizontally to multiple container replicas behind a load balancer without sticky sessions, each replica maintains its own bucket. A distributed store (e.g. Redis) can replace the in-memory engine when distributed scaling is required.
