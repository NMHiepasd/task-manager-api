# ADR-009: CI/CD Pipeline, Security Scanning, and Container Registry Packaging

## Status
Accepted

## Context
A production-grade REST API requires automated quality verification, continuous security audits, and immutable artifact delivery on every codebase change. Specifically:
1. Every commit and pull request must undergo automated static analysis (`golangci-lint`), race detection (`-race`), and supply-chain vulnerability audits (`govulncheck`).
2. Database schema migrations must be verified for bidirectional idempotency (up -> down -> up) against a clean database instance in CI.
3. Container artifacts deployed to production must follow immutable tagging conventions (commit SHA and semver) rather than mutable `:latest` tags, preventing unpredictable state across distributed environments.
4. Container images must undergo vulnerability scanning (OS packages and application dependencies) before reaching the registry.

## Decision

### 1. GitHub Actions Multi-Stage Workflow
We establish an automated pipeline in `.github/workflows/ci.yml` composed of four parallel and sequential jobs:
- **`lint`**: Executes `golangci/golangci-lint-action` with caching via `actions/setup-go@v5`.
- **`security`**: Executes official `govulncheck` from the Go security team to detect known Go standard library and dependency disclosures.
- **`test`**:
  - Provisions ephemeral **Service Containers** for `PostgreSQL 16` and `Redis 7` on the GitHub runner.
  - Executes `scripts/test-migrations.sh` to guarantee migration reversibility on a clean database.
  - Runs unit tests with `-race` detection, integration tests with Testcontainers, and fuzz testing for cursor pagination.
- **`delivery`** (runs only after `lint`, `security`, and `test` pass):
  - Builds the container using Docker Buildx and GitHub Actions layer caching (`type=gha`).
  - Scans the resulting container image using **Trivy** (`aquasecurity/trivy-action`), failing the pipeline on unpatched `HIGH` or `CRITICAL` vulnerabilities.
  - Pushes images to **GitHub Container Registry (`ghcr.io/nmhiepasd/task-manager-api`)** exclusively on pushes to the `main` branch.

### 2. Immutable Container Tagging Policy
We strictly adhere to the project constraint: **never use the `:latest` tag**.
- Images are tagged with:
  - Short Git commit SHA: `sha-<short_sha>` (e.g., `sha-0a4e102`)
  - Semantic version (when Git tags are pushed): `vX.Y.Z`, `X.Y`
  - Active branch name: `main`
- Configuration: `flavor: latest=false` in `docker/metadata-action`.

### 3. Automated Dependency Lifecycle
We configure `.github/dependabot.yml` to schedule weekly dependency monitoring and pull requests for:
- `gomod` (Go modules)
- `docker` (Alpine runtime and Golang builder base images)
- `github-actions` (Workflow action versions)

## Consequences

### Positive
- **Deterministic Deployments**: Deploying specific commit SHAs prevents container drift and makes rollbacks instant and reliable.
- **Supply-Chain & Container Hardening**: Two layers of CVE scanning (source-level `govulncheck` and container-level `Trivy`) ensure zero unvetted vulnerabilities enter production.
- **Fast Feedback**: GitHub Actions Go caching and Docker Buildx layer caching minimize pipeline execution time.

### Negative / Trade-offs
- CI pipeline consumes GitHub Actions minutes on each push/PR.
- Dependabot pull requests require periodic review and merging by maintainers.
