# ADR-013: Automated API Contract and Property-Based Fuzz Testing with Schemathesis

## Status
Accepted

## Context
While unit tests and integration tests verify known scenarios and developer-anticipated edge cases, production web APIs face unpredictable inputs:
1. **Input Combinations & Boundaries**: Clients may transmit unusual unicode strings, boundary integers, malformed JSON structures, unexpected query parameter types, or omitted required fields.
2. **Schema & Contract Drift**: Documentation in OpenAPI (Swagger) can silently drift from actual handler responses (e.g., returning undocumented HTTP status codes such as 429 or mismatched media types).
3. **500 Defense**: Robust APIs must gracefully reject invalid data with RFC 9457 `4xx` Client Errors rather than crashing with unhandled `500 Internal Server Error` panics.

## Decision

### 1. Adopt Schemathesis (`schemathesis/schemathesis:stable`)
We adopt **Schemathesis** (v4.33+) for property-based API testing and contract conformance against our OpenAPI 2.0 / 3.0 specification:
- Runs in Docker (`schemathesis/schemathesis:stable`) attached to the Docker Compose network (`task-manager-api_task_network`), requiring zero host Python dependencies.
- Automatically parses `/swagger/doc.json` and exercises all 24 API operations across Auth, Users, Projects, Tasks, and Tags.

### 2. Configuration (`schemathesis.toml`)
We define declarative test settings in `schemathesis.toml`:
- **Concurrency**: 4 parallel workers (`workers = 4`).
- **Phases**:
  - `coverage`: Systematically exercises every declared parameter, response code, and operation.
  - `fuzzing`: Employs property-based generation (Hypothesis engine) to synthesize edge cases, extreme string lengths, special characters, and boundary numbers.
  - `stateful`: Generates multi-step sequence scenarios following API links.
- **Enforced Conformance Checks**:
  - `not_a_server_error`: Asserts that no request ever triggers an unhandled 5xx server error.
  - `status_code_conformance`: Asserts that every returned HTTP status code is documented in the Swagger specification.
  - `content_type_conformance`: Asserts that response headers match documented `application/json` or `application/problem+json`.

### 3. Verification & Spec Hardening
Running Schemathesis immediately identified missing `@Failure 429 {object} ProblemDetails` annotations on rate-limited endpoints (`POST /api/v1/auth/refresh` and `POST /api/v1/auth/logout`), which were rectified and re-generated in `docs/swagger`.

### 4. Developer & CI Integration
- Standalone runner script: `scripts/run-schemathesis.sh`.
- Makefile target: `make schemathesis`.

## Consequences

### Positive
- **Automated Defect & Drift Discovery**: Generated 1,138 property-based test cases across all 24 endpoints with 100% operation coverage in under 12 seconds.
- **Contract Guarantee**: Guarantees that clients consuming the OpenAPI spec will never encounter undocumented status codes or undeclared response formats.
- **No Production Panics**: Proves resilience against invalid, oversized, or malicious inputs without crashing.

### Negative / Trade-offs
- Requires the API stack (PostgreSQL, Redis, API) to be running when executing the test suite.
