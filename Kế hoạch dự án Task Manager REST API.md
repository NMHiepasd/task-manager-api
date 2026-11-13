# Kế hoạch dự án: Task Manager REST API

Stack: Go, Gin, PostgreSQL, Docker. Mỗi phase có **mục tiêu**, **task chi tiết** và **Definition of Done (DoD)**. Chỉ sang phase sau khi đạt DoD của phase trước. Phiên bản thư viện nên kiểm tra bản mới nhất khi cài.

## Cấu trúc thư mục

```
taskmgr/
├── cmd/api/main.go            # entrypoint: load config, wire deps, start server
├── internal/
│   ├── config/                # đọc env
│   ├── transport/http/        # gin: router, handler, middleware, DTO (chỉ tầng này import gin)
│   ├── service/               # business logic (interface + implementation)
│   ├── repository/            # sqlc generated + wrapper
│   ├── domain/                # entity, error types
│   └── platform/              # postgres pool, redis, logger
├── db/
│   ├── migrations/            # file .sql up/down
│   └── queries/               # file .sql cho sqlc
├── api/openapi.yaml
├── deployments/               # Dockerfile, compose
├── docs/                      # ADR, ERD
├── scripts/
├── Makefile
├── sqlc.yaml
└── .golangci.yml
```

## Phase 0: Chuẩn bị repo và môi trường

**Mục tiêu:** có bộ khung để mọi commit sau đều được lint, format, kiểm tra tự động.

- [x] Tạo repo GitHub, `go mod init`, chọn Go version tối thiểu và ghi vào `go.mod`
- [x] Tạo cấu trúc thư mục như trên
- [x] `.gitignore` (có `.env`, `bin/`, `tmp/`) và `.env.example`
- [x] Makefile với các target: `run`, `build`, `test`, `lint`, `migrate-up`, `migrate-down`, `sqlc`, `docker-up`
- [x] `.golangci.yml` bật: `govet`, `staticcheck`, `errcheck`, `gosec`, `gofumpt`, `revive`, `bodyclose`, `sqlclosecheck`
- [x] Cài `air` (hot reload) và `lefthook` (pre-commit chạy lint + test nhanh)
- [x] Quy ước Conventional Commits (`feat:`, `fix:`, `chore:`)
- [x] README khung và thư mục `docs/adr/` với ADR-001 "Vì sao chọn Gin + sqlc + pgx"

**DoD**

- [x] `make lint` và `make test` chạy được (dù chưa có test)
- [x] Commit đầu tiên đã lên GitHub

## Phase 1: Khung ứng dụng, Docker, Database

**Mục tiêu:** `docker compose up` chạy được app + Postgres, có `/healthz`, migration chạy được.

### 1.1 Config và logger

- [x] `caarlos0/env/v11` đọc env vào struct (`APP_PORT`, `DB_URL`, `JWT_SECRET`, `GIN_MODE`, ...), validate khi khởi động, thiếu là fail ngay
- [x] `log/slog` với JSON handler, level theo env

### 1.2 Database

- [x] Docker Compose: service `postgres` (có healthcheck, volume) và `api` (depends_on healthy)
- [x] `pgx/v5/pgxpool`: cấu hình `MaxConns`, `MinConns`, `MaxConnLifetime`, `HealthCheckPeriod`
- [x] `golang-migrate`: migration đầu tiên tạo extension và bảng `users`
- [x] Thiết kế schema và vẽ ERD (dbdiagram.io hoặc Mermaid): `users`, `projects`, `tasks`, `tags`, `task_tags`, `refresh_tokens`
- [x] Khóa chính UUID (`google/uuid`, ưu tiên v7)
- [x] Dùng `timestamptz`, các cột `created_at`, `updated_at`, `deleted_at`, và cột `version` (optimistic lock)
- [x] Constraint: `NOT NULL`, `CHECK` cho status, `UNIQUE (lower(email))`
- [x] Cấu hình `sqlc.yaml` (engine postgresql, sql_package `pgx/v5`), viết thử 1-2 query và sinh code

### 1.3 HTTP server khung

- [x] `gin.New()`, `gin.SetMode` theo config
- [x] Hàm `NewRouter(deps) *gin.Engine` (để test được)
- [x] `http.Server` riêng với `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`
- [x] Graceful shutdown bằng `signal.NotifyContext` + `server.Shutdown`
- [x] `/healthz` (liveness) và `/readyz` (ping DB)
- [x] `SetTrustedProxies`

### 1.4 Dockerfile

- [x] Multi-stage: build với `golang:<version>`, chạy với `distroless` hoặc `alpine`, non-root user
- [x] Build flag: `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=..."`

**DoD**

- [x] `docker compose up` chạy thành công
- [x] `curl /readyz` trả 200
- [x] `make migrate-up` / `make migrate-down` hoạt động
- [x] Shutdown bằng Ctrl+C không làm mất request đang chạy

## Phase 2: Middleware nền và xử lý lỗi thống nhất

**Mục tiêu:** mọi request đều có ID, log chuẩn, lỗi trả đúng một format.

- [x] Middleware request ID (`gin-contrib/requestid` hoặc tự viết), gắn vào log và header response
- [x] Middleware logger `slog`: method, path, status, latency, request_id, client_ip, bỏ qua `/healthz`
- [x] Middleware recovery tùy chỉnh: log stack, trả JSON 500 chung chung
- [x] Error model trong `domain/`: `ErrNotFound`, `ErrConflict`, `ErrUnauthorized`, `ErrForbidden`, `ErrValidation`
- [x] Error-handling middleware: handler gọi `c.Error(err)`, middleware map error sang status + body theo RFC 9457 (`type`, `title`, `status`, `detail`, `code`, `request_id`)
- [x] `NoRoute`, `NoMethod` (`HandleMethodNotAllowed = true`) trả JSON cùng format
- [x] Hàm chuyển `validator.ValidationErrors` thành danh sách lỗi theo field
- [x] Giới hạn body bằng `http.MaxBytesReader`
- [x] Tắt `RedirectTrailingSlash` / `RedirectFixedPath` và ghi vào ADR
- [x] Tạo group `/api/v1`

**DoD**

- [x] Gọi route không tồn tại, gửi JSON sai, gây panic thử: đều ra response cùng cấu trúc
- [x] Có `request_id` trong log của các trường hợp trên

## Phase 3: Xác thực và phân quyền

**Mục tiêu:** đăng ký/đăng nhập an toàn, mọi route private đều biết user hiện tại.

**Endpoint:** `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /me`

- [x] Tầng `service/auth`: interface + implementation, nhận repository qua constructor
- [x] Hash mật khẩu bằng `bcrypt` (cost >= 12) hoặc `argon2id`
- [x] JWT bằng `golang-jwt/jwt/v5`: access token ngắn (10-15 phút), kiểm tra `alg`, `exp`, `iss`, `aud`
- [x] Refresh token: sinh chuỗi ngẫu nhiên (`crypto/rand`), lưu hash trong DB, rotation mỗi lần refresh, revoke khi logout
- [x] Middleware `AuthRequired`: parse Bearer token, set `userID` vào context bằng key hằng số, helper `CurrentUserID(c)`
- [x] Thông báo lỗi đăng nhập chung chung, không lộ email có tồn tại hay không
- [x] DTO riêng cho request/response (không bind thẳng vào model)
- [x] Rate limit riêng cho `/auth/login` và `/auth/register` (`x/time/rate` hoặc `ulule/limiter`)
- [x] Validator rule: email, độ dài/độ mạnh mật khẩu
- [x] (Tùy chọn) RBAC đơn giản: cột `role`, middleware `RequireRole("admin")`

**DoD**

- [x] Test đăng ký trùng email
- [x] Test sai mật khẩu
- [x] Test token hết hạn
- [x] Test refresh token dùng lại lần hai bị từ chối

## Phase 4: Domain chính: Projects, Tasks, Tags

**Mục tiêu:** CRUD đầy đủ, đúng quyền sở hữu, query tối ưu.

### 4.1 Projects

- [x] `POST/GET/PATCH/DELETE /projects`

### 4.2 Tasks

- [x] `POST/GET/PATCH/DELETE /tasks` và `GET /projects/:id/tasks`
- [x] Các trường: `title`, `description`, `status`, `priority`, `due_date`, `project_id`

### 4.3 Tags

- [x] CRUD tag
- [x] `PUT /tasks/:id/tags` (quan hệ nhiều-nhiều)

### 4.4 Task kỹ thuật chung

- [x] Viết query trong `db/queries/*.sql`, chạy `sqlc generate`
- [x] Mọi query đều có `WHERE user_id = $1` (chống IDOR); resource không thuộc về user thì trả 404 thay vì 403
- [x] Soft delete và partial index `WHERE deleted_at IS NULL`
- [x] Optimistic locking: `UPDATE ... WHERE id=$1 AND version=$2`, 0 row thì trả 409 Conflict
- [x] Transaction cho thao tác nhiều bước (helper `WithTx(ctx, fn)`)
- [x] Keyset pagination: `?cursor=...&limit=20`, trả `next_cursor`, giới hạn `limit` tối đa
- [x] Lọc và sắp xếp: `status`, `priority`, `due_before`, `tag`, `q` (thử `ILIKE` trước, sau đó nâng lên full-text search `tsvector` + GIN index)
- [x] Binding bằng `ShouldBindJSON`, `ShouldBindQuery`, `ShouldBindUri`
- [x] Custom validator (ví dụ `due_date` không ở quá khứ, status hợp lệ)
- [x] Truyền `c.Request.Context()` xuống mọi query
- [x] Thêm index theo pattern truy vấn (composite `(user_id, status, due_date)`, ...)
- [ ] Seed data script (`gofakeit`) tạo khoảng 100k task để phase tối ưu có dữ liệu thử

**DoD**

- [x] User A không đọc/sửa/xóa được dữ liệu của user B (có test chứng minh)
- [x] Hai request sửa đồng thời cho ra một 409

## Phase 5: Testing

Làm xen kẽ từ Phase 2, chốt ở phase này.

- [x] Unit test service bằng `testify` + mock (`uber-go/mock` hoặc `mockery`), dùng table-driven test
- [x] Integration test repository với `testcontainers-go` (Postgres thật, chạy migration trước), tách bằng build tag `//go:build integration`
- [x] HTTP test với `httptest` + `NewRouter()`, đặt `gin.SetMode(gin.TestMode)` trong `TestMain`
- [x] Test các case bảo mật: IDOR, token giả/hết hạn, mass assignment
- [x] `go test -race ./...` luôn bật trong CI; thêm `goleak` cho package có goroutine
- [x] Coverage mục tiêu hợp lý (khoảng 70% cho service, không chạy theo con số)
- [x] Fuzz test cho hàm parse cursor/validate input (`go test -fuzz`)
- [x] Collection Bruno hoặc Hurl để test thủ công và smoke test, commit vào repo
- [x] Target Makefile: `test`, `test-integration`, `test-race`, `cover`

**DoD**

- [x] `make test-integration` chạy được trên máy sạch chỉ cần Docker

## Phase 6: Tài liệu API

- [x] Chọn một hướng: `swaggo/swag` (comment trong handler) hoặc OpenAPI viết tay + `oapi-codegen`
- [x] Gắn `gin-swagger` ở `/swagger` (chỉ bật ở dev/staging)
- [x] Mô tả đủ: auth scheme, mã lỗi, ví dụ request/response
- [x] Cập nhật README: kiến trúc, ERD, cách chạy, danh sách biến môi trường

**DoD**

- [x] Thử được toàn bộ endpoint từ Swagger UI
- [x] Người mới đọc README có thể chạy được dự án

## Phase 7: Hardening bảo mật và độ tin cậy

- [x] `gin-contrib/cors` cấu hình whitelist origin cụ thể
- [x] `gin-contrib/secure` thêm security headers
- [x] Rate limit toàn cục theo IP/user (kết hợp Redis ở phase sau)
- [x] Idempotency-Key cho `POST /tasks` (bảng DB hoặc Redis, TTL 24h)
- [x] Timeout cho request (`context.WithTimeout`) và cho query DB
- [x] Rà soát theo OWASP API Security Top 10, ghi kết quả vào `docs/security.md`
- [x] Chạy `govulncheck`, `gosec`, `gitleaks`, `trivy image` và sửa các cảnh báo
- [x] Rà soát log: không có mật khẩu/token/email thô
- [x] Quản lý secret: không hardcode, `.env` không commit

**DoD**

- [x] Các công cụ quét không còn lỗi mức High/Critical

## Phase 8: Cache và tối ưu hiệu năng

Làm theo vòng: đo, tìm nút thắt, sửa, đo lại.

### 8.1 Đo trước

- [x] Bật `pg_stat_statements`, `log_min_duration_statement=200`, `auto_explain`
- [x] Kịch bản k6 cho: login, list tasks, tạo task (ghi lại p95, RPS, error rate làm baseline)
- [x] `net/http/pprof` ở cổng nội bộ riêng

### 8.2 Tối ưu DB

- [x] `EXPLAIN (ANALYZE, BUFFERS)` cho các query chính, thêm/sửa index, tạo index bằng `CONCURRENTLY`
- [x] Rà soát N+1, giảm số round-trip (dùng JOIN hoặc `pgx.Batch`)
- [x] Chỉnh pool size và ghi lại kết quả trước/sau

### 8.3 Cache

- [x] Thêm Redis vào Compose, dùng `go-redis/v9`
- [x] Cache-aside cho dữ liệu đọc nhiều (danh sách tag, thống kê project), có TTL và invalidate khi ghi
- [x] `singleflight` chống cache stampede
- [x] Chuyển rate limiter sang Redis để chạy được nhiều instance
- [x] `gin-contrib/gzip` nén response, thử thêm ETag cho GET

### 8.4 Benchmark

- [x] `go test -bench` + `benchstat` cho hàm nóng
- [x] Chỉ thử `-tags=jsoniter` / `sonic` nếu profile cho thấy JSON là nút thắt

**DoD**

- [x] Có bảng so sánh trước/sau (p95, RPS) trong `docs/performance.md`

## Phase 9: CI/CD và đóng gói

- [x] GitHub Actions: `lint`, `test -race` (service Postgres/Redis hoặc testcontainers), `govulncheck`, `docker build`, `trivy`, push image lên GHCR
- [x] Image tag theo commit SHA/semver, không dùng `latest`
- [x] Cache module Go trong CI
- [x] Test migration up/down trên DB sạch trong CI
- [x] Dependabot hoặc Renovate
- [x] `docker-compose.prod.yml` + Caddy (TLS tự động), secret qua env/Docker secrets
- [x] Script backup `pg_dump` định kỳ và thử restore thật một lần, ghi lại các bước
- [x] Deploy thử lên VPS giá rẻ
- [x] Badge CI và `CHANGELOG` bằng `git-cliff`

**DoD**

- [x] Push lên `main` tự build và scan
- [x] Có thể deploy lại bằng một lệnh

## Phase 10: Nâng cấp (tùy chọn)

- [x] Background job (nhắc deadline, dọn token hết hạn): `river` (Postgres) hoặc `asynq` (Redis)
- [x] Metrics: `prometheus/client_golang` + Grafana (latency, RPS, error rate)
- [x] Tracing: OpenTelemetry + `otelgin` + `otelpgx`
- [ ] Alerting: Alertmanager hoặc Grafana Alerting
- [ ] Kiểm thử API tự động từ spec: `schemathesis`
- [ ] Tính năng thêm: comment, file đính kèm (S3/MinIO), chia sẻ project, webhook

## Nguyên tắc xuyên suốt

- [ ] Mỗi tính năng là một nhánh + một PR (kể cả khi làm một mình), CI xanh mới merge
- [ ] Viết test cùng lúc với code, đặc biệt là test phân quyền
- [ ] Mỗi quyết định kỹ thuật quan trọng viết một ADR ngắn (5-10 dòng)
- [ ] Chỉ thêm công cụ khi gặp đúng vấn đề nó giải quyết
- [ ] Cuối mỗi phase, tag version (`v0.1.0`, `v0.2.0`, ...)

## Checklist nhanh về Gin (dán vào repo)

- [ ] `GIN_MODE=release` ở production
- [ ] `gin.New()` + logger structured + recovery riêng
- [ ] `http.Server` có đủ timeout + graceful shutdown
- [ ] `SetTrustedProxies` đã cấu hình đúng
- [ ] Giới hạn kích thước body
- [ ] Dùng `ShouldBind*` + DTO tách khỏi model DB
- [ ] `c.Copy()` khi dùng goroutine, truyền `c.Request.Context()` xuống DB
- [ ] Sau `Abort` luôn `return`
- [ ] Error middleware + `NoRoute`/`NoMethod` trả JSON thống nhất
- [ ] Route nằm trong `/api/v1`, middleware gắn theo group
- [ ] Build tag và flag build ghi trong `Makefile`/`Dockerfile`
- [ ] `go.mod`/`go.sum` được commit, chạy `govulncheck` trong CI
- [ ] Service/repository không import `gin`
