# ADR-005: Testing Strategy, Testcontainers, and Build Separation

## Status
Accepted

## Context
A reliable backend service requires testing across multiple levels of the test pyramid:
1. **Unit Testing**: Fast, isolated tests mocking dependencies to test domain logic, error branches, and edge cases in milliseconds.
2. **Integration Testing**: Testing actual interactions with external systems—specifically PostgreSQL—to verify complex queries, database constraints, transactions, and indexing behavior.
3. **Robustness & Memory Safety**: Verifying that untrusted user input does not panic the service (fuzz testing) and that concurrent processes do not leak goroutines or trigger data races.

Relying solely on mocked unit tests fails to detect SQL syntax errors, broken database constraints (e.g. check constraints or foreign keys), transactions that fail to roll back, or full-text search indexing bugs. Conversely, relying on a shared external database introduces flakiness and state pollution between test runs.

## Decision

### 1. Test Pyramid & Tooling
We adopt a multi-tier testing strategy:
- **Unit Testing**: Implemented with `testify` (`assert` & `require`) and table-driven tests. Mock interfaces are used for fast feedback (< 2 seconds for the entire unit test suite).
- **Integration Testing with Testcontainers**: Implemented using `testcontainers-go` (`modules/postgres`). A real `postgres:16-alpine` container is spun up dynamically for integration tests, automatically applying all migration SQL files before running test assertions against real database tables.
- **Build Tag Separation**: All integration tests reside with the `//go:build integration` build directive. This guarantees standard `go test ./...` and git pre-commit hooks remain lightning fast, while `go test -tags=integration ./...` executes the full database integration suite.
- **Fuzz Testing**: Implemented using Go's built-in fuzzing engine (`go test -fuzz`) for opaque pagination cursor decoding (`pagination.DecodeCursor`), guaranteeing zero crashes, panics, or memory exhaustion when processing malformed or malicious client cursors.
- **Race Detection & Goroutine Leak Prevention**: Verified using Go's race detector (`-race`) and `go.uber.org/goleak` in packages with concurrent structures (such as the IP rate limiter).
- **API Smoke Testing Collection**: A Bruno collection committed under `bruno/` for developer exploratory testing and endpoint verification.

### 2. Makefile Automation
We provide distinct, purpose-built Makefile targets:
- `make test`: Runs unit tests with `-short`.
- `make test-race`: Runs unit tests with `-race` to catch concurrency bugs.
- `make test-integration`: Runs integration tests against Dockerized PostgreSQL via Testcontainers.
- `make test-fuzz`: Runs fuzz testing for cursor pagination.
- `make cover`: Generates an HTML code coverage report.

## Consequences

### Positive
- **High Confidence**: Database transactions (`WithTx`), Full-Text Search GIN index queries, and keyset pagination tuple comparisons are validated against real PostgreSQL instances.
- **Zero Test Pollution**: Testcontainers dynamically creates and destroys isolated test databases, preventing shared state bugs.
- **Fast Developer Workflow**: Unit tests execute in under 2 seconds without requiring Docker daemon interaction, keeping `lefthook` pre-commit hooks fast.
- **Input Resilience**: Fuzzing ensures robust error handling against arbitrary input bytes.

### Negative / Trade-offs
- Integration tests require a running Docker daemon and take ~5 seconds to spin up and tear down containers. This is why they are isolated behind `-tags=integration`.
