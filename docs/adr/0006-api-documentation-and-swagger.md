# ADR-006: API Documentation with OpenAPI 3.0 and Swagger UI

## Status
Accepted

## Context
A production REST API requires accessible, up-to-date, and interactive documentation for frontend developers, third-party consumers, and QA engineers.
There are two main strategies for API documentation in Go:
1. **Design-First (Spec-First)**: Hand-authoring OpenAPI YAML/JSON files and generating Go boilerplate with `oapi-codegen`.
2. **Code-First**: Annotating Go handler functions directly and generating OpenAPI specifications using `swaggo/swag` and mounting `gin-swagger`.

Writing OpenAPI YAML by hand creates cognitive overhead and easily falls out of sync with actual Go struct definitions and validation tags as the API evolves. On the other hand, maintaining declarative annotations directly above Gin handlers keeps route documentation, HTTP status codes, and DTO schemas colocated with code.

## Decision
We adopt **Code-First documentation with `swaggo/swag` and `gin-swagger`**:

1. **Declarative Annotations**:
   - General API metadata (title, version, description, Bearer auth scheme) is declared in `cmd/api/main.go`.
   - Each HTTP handler function includes standard Swag annotations: `@Summary`, `@Description`, `@Tags`, `@Accept`, `@Produce`, `@Param`, `@Success`, `@Failure`, `@Router`, and `@Security`.
2. **RFC 9457 Error Schema Standardization**:
   - All failure responses reference `domain.ProblemDetails` (via `handler.ProblemDetails`), ensuring consistent error documentation (`application/problem+json`) across all 4xx/5xx responses.
3. **Internal Package & Dependency Resolution**:
   - Spec generation is configured with `--parseInternal` and `--parseDependency` to allow `swag` to traverse internal service and domain packages for request/response DTO resolution.
4. **Interactive Swagger UI Mounting**:
   - Swagger UI is mounted at `GET /swagger/*any` via `github.com/swaggo/gin-swagger` and `github.com/swaggo/files`.
5. **Makefile Automation**:
   - A `make swagger` target is added to regenerate OpenAPI artifacts (`docs/swagger/docs.go`, `swagger.json`, `swagger.yaml`) on demand.

## Consequences

### Positive
- **Single Source of Truth**: Route definitions, request DTOs, and response schemas remain colocated with handler implementations.
- **Interactive Testing**: Developers can authenticate with JWT Bearer tokens and execute live HTTP requests directly from the browser UI at `/swagger/index.html`.
- **Exportable Specifications**: Generates standard `swagger.json` and `swagger.yaml` which can be imported into Postman, Bruno, or automated contract testing tools.

### Negative / Trade-offs
- Handler files contain multiline doc comments before each handler function.
- Changing struct fields requires re-running `make swagger` before deploying.
