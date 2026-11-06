# ADR-002: Disable Trailing Slash and Fixed Path Redirects in HTTP Router

## Context
By default, the Gin framework enables:
1. `RedirectTrailingSlash`: Automatically redirects URLs with a trailing slash to their counterpart without a slash (e.g., `/api/v1/tasks/` -> `/api/v1/tasks`) via HTTP 301/307/308 redirects.
2. `RedirectFixedPath`: Attempts to fix URL case and clean redundant path elements (e.g., `/API//v1/tasks` -> `/api/v1/tasks`).

While convenient for traditional multi-page web applications, automatic redirects in a production REST API introduce several architectural and security issues:
- **Header Stripping / Method Mismatch**: When clients make a `POST`, `PUT`, or `DELETE` request, following an HTTP 301/302 redirect can result in method transformation to `GET` (as per legacy HTTP clients) or stripping of sensitive headers like `Authorization: Bearer ...`.
- **Latency**: An extra HTTP round-trip redirect adds network latency for client applications.
- **Strict Contract Enforcement**: REST APIs should enforce explicit, deterministic URI paths. Non-canonical URLs should fail fast with `404 Not Found` rather than implicitly guessing client intent.
- **Security Boundary**: Path mutations can lead to URL normalization confusion in reverse proxies, API gateways, and authorization filters.

## Decision
We explicitly set:
```go
router.RedirectTrailingSlash = false
router.RedirectFixedPath = false
router.HandleMethodNotAllowed = true
```

- Any request with a trailing slash that is not explicitly registered will return an RFC 9457 `404 Not Found` response.
- Any request with an unsupported HTTP method for an existing route will return an RFC 9457 `405 Method Not Allowed` response.

## Consequences
- **Positive**: Strict, predictable routing contract; no hidden round-trips or authorization header loss.
- **Positive**: Complies with RFC 9110 and RFC 9457.
- **Trade-off**: Clients must adhere strictly to the exact registered API routes without trailing slashes.
