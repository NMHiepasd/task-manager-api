# Kế hoạch v1.2.1: Sửa lỗi v1.2.0 và hoàn thiện

Nguồn: review HEAD sau v1.1.1 (88/100). Vì v1.2.0 chưa được tag nên các bản sửa sẽ phát hành thành v1.2.1 (xem U3). Không kèm thời gian dự kiến.

## Quy ước chung (giữ như kế hoạch v1.1 và v1.1.1)

- Không đọc hay sửa file sinh tự động (`internal/repository/*.sql.go`, `docs/swagger/*`): sửa nguồn rồi chạy `make sqlc` / `make swagger`
- Viết test tái hiện trước, sửa sau (đỏ rồi xanh)
- Chạm tối đa khoảng 8 file; vượt thì dừng, báo cáo và đề xuất tách task
- Thêm biến env thì cập nhật cùng lúc `.env.example`, `docker-compose.prod.yml` và bảng env trong README
- Migration mới đánh số tiếp theo (000012, 000013), luôn có file down và chạy `scripts/test-migrations.sh`
- Trước khi commit: `make lint` và `go test -race -short ./...`; task có SQL thì thêm `make test-integration`
- Việc agent không chạy được (docker compose thật, nhiều replica) phải ghi rõ trong báo cáo để người dùng tự kiểm tra
- Không dời hay xóa tag đã phát hành
- Cuối task: tick checklist trong tài liệu này, commit theo Conventional Commits

Mẫu prompt:

```
Đọc docs/improvement-plan-v1.2.1.md. Chỉ thực hiện Task P1, không làm task khác.
Chỉ chạm các file liệt kê trong task. Viết test tái hiện trước khi sửa.
Chạy lệnh kiểm tra, tick checklist, commit rồi báo cáo: file đã đổi, kết quả test, việc cần người dùng tự kiểm tra.
```

## Thứ tự và phụ thuộc

- Ưu tiên cao: giai đoạn P, Q1, R1 và R2
- Trong P: P1 → P2 → P3 → P4 → P5
- Trong Q: Q1 → Q2; Q3a → Q3b
- Trong R: R1 → R2 → R3 → R4
- Trong S: S1 → S2 → S3 → S4 → S5; S3 làm sau Q1 (mở khóa dùng cột khóa tài khoản)
- Giai đoạn T độc lập; T4 nên làm trước U1
- Giai đoạn U làm cuối cùng

## Giai đoạn P: Outbox giao đúng một lần (ưu tiên cao)

### P1. Migration và query claim có lease

**Mục tiêu:** nhiều relay không bao giờ lấy trùng một sự kiện.

**File:** `migrations/000012_outbox_claim_lease.up.sql` và `.down.sql`, `queries/outbox.sql`, test integration mới.

- [x] Thêm trạng thái `processing` vào CHECK, cột `locked_at TIMESTAMPTZ`, index một phần cho `processing`
- [x] Query `ClaimOutboxEvents(limit, max_retries)`: một câu `UPDATE ... SET status = 'processing', locked_at = NOW() WHERE id IN (SELECT ... FOR UPDATE SKIP LOCKED) RETURNING ...`, không còn SELECT FOR UPDATE chạy ngoài transaction
- [x] Query `ReclaimStaleOutboxEvents(lease_seconds)`: đưa dòng `processing` quá hạn về `pending`
- [x] Mark published, retry và failed chỉ áp dụng cho dòng đang `processing`
- [x] Test integration: hai goroutine claim cùng lúc nhận hai tập rời nhau

**Xong khi:** `make sqlc` thành công, test xanh, migration up/down chạy được.

### P2. Relay dùng claim và deliveryID xác định

**File:** `internal/worker/outbox_relay.go`, `internal/worker/distributor.go`, `internal/worker/outbox_relay_test.go`.

- [ ] Thay `FetchPendingOutboxEvents` bằng `ClaimOutboxEvents`
- [ ] `deliveryID` sinh bằng `uuid.NewSHA1` từ (id sự kiện outbox, id webhook) thay cho `uuid.New()`
- [ ] Enqueue kèm `asynq.TaskID(deliveryID)`; coi `asynq.ErrTaskIDConflict` là thành công
- [ ] Trường `timestamp` của envelope lấy từ `created_at` của sự kiện
- [ ] Test: cùng một sự kiện xử lý hai lần cho cùng deliveryID, `ErrTaskIDConflict` không bị tính là lỗi

**Xong khi:** test xanh, không còn `uuid.New()` trong luồng relay.

### P3. Lỗi từng phần và phục hồi lease

**File:** `internal/worker/outbox_relay.go`, `internal/config/config.go`, test.

- [ ] Enqueue lỗi ở webhook thứ k thì sự kiện quay về `pending` (retry\_count tăng); các webhook đã enqueue không bị trùng nhờ ID xác định
- [ ] Mỗi lượt poll gọi `ReclaimStaleOutboxEvents` với `OUTBOX_LEASE_SECONDS` (mặc định 120)
- [ ] Test: relay dừng đột ngột sau khi claim (chưa mark) thì lượt sau reclaim và xử lý, tổng cộng đúng một lần

**Xong khi:** test xanh.

### P4. Test tích hợp nhiều relay

**File:** `internal/worker/outbox_relay_integration_test.go` (build tag `integration`).

- [ ] Postgres thật (testcontainers) và distributor giả ghi nhận lời gọi; tạo 200 sự kiện, chạy 3 relay song song
- [ ] Khẳng định mỗi cặp (sự kiện, webhook) chỉ có đúng một task hiệu lực
- [ ] Kịch bản crash giữa enqueue và mark published
- [ ] Chạy với `-race`

**Xong khi:** `make test-integration` xanh. Nếu phát hiện lỗi mới thì ghi thành task, không sửa lan.

### P5. Cờ bật relay, ADR và tài liệu

**File:** `cmd/api/main.go`, `internal/config/config.go`, `docs/adr/0020-transactional-outbox-pattern.md`, `docs/webhooks.md`, `.env.example`, README.

- [ ] Env `OUTBOX_RELAY_ENABLED` (mặc định true), `OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`; ghi vào `.env.example`, compose và README
- [ ] ADR-0020: giao at-least-once và khử trùng bằng deliveryID, không bảo đảm thứ tự, cơ chế lease, dead-letter
- [ ] `docs/webhooks.md`: khuyến nghị bên nhận loại trùng theo `X-Webhook-Id`

**Xong khi:** tài liệu khớp code.

## Giai đoạn Q: Khóa tài khoản

### Q1. Sửa bộ đếm khóa tài khoản (ưu tiên cao)

**File:** `queries/users.sql`, `internal/service/auth_service.go`, test service và integration.

- [ ] `RecordFailedLogin` nhận tham số threshold, duration, window; nếu `last_failed_login_at` cũ hơn window hoặc khóa đã hết hạn thì đặt bộ đếm về 1 rồi mới tính
- [ ] Test integration: sai 4 lần rồi để qua window, sai thêm 1 lần thì không bị khóa; khóa hết hạn rồi sai 1 lần thì bộ đếm bằng 1
- [ ] Log lỗi của `RecordFailedLogin` thay cho `_, _ =`

**Xong khi:** hành vi khớp mô tả trong ADR-0021.

### Q2. Đưa ngưỡng ra config

**File:** `internal/config/config.go`, `internal/service/auth_service.go`, `.env.example`, `docker-compose.prod.yml`, README.

- [ ] `AUTH_LOCKOUT_THRESHOLD` (5), `AUTH_LOCKOUT_DURATION` (15m), `AUTH_LOCKOUT_WINDOW` (15m)
- [ ] `Validate`: threshold từ 3 trở lên, duration và window trong khoảng hợp lý
- [ ] Test config

**Xong khi:** không còn số 5 hay 15 phút cố định trong SQL.

### Q3a. ADR chống dò email và khóa tài khoản làm DoS

**File:** `docs/adr/0022-login-response-and-lockout-abuse.md`.

- [ ] So sánh: (a) trả cùng thông điệp 401 cho tài khoản bị khóa, (b) khóa theo cặp email và IP, (c) tăng dần độ trễ; chọn một, ghi lý do và rủi ro còn lại
- [ ] Quy định log bảo mật và cách báo cho người dùng hợp lệ

**Xong khi:** chỉ có file ADR.

### Q3b. Áp dụng chính sách phản hồi đăng nhập

**Điều kiện:** sau Q3a.

**File:** `internal/service/auth_service.go`, handler auth, test, swagger, README.

- [ ] Sửa `Login` theo ADR-0022; email không tồn tại, sai mật khẩu và (nếu chọn phương án a) bị khóa trả cùng body và mã lỗi
- [ ] Test: ba tình huống cho cùng phản hồi và thời gian xấp xỉ nhờ hash giả
- [ ] `make swagger`

**Xong khi:** test xanh, không còn mã 423 lộ ra nếu ADR chọn phương án a.

## Giai đoạn R: Mã hóa secret webhook

### R1. Định dạng, AAD và key ring trong `pkg/crypto`

**File:** `pkg/crypto/aes_gcm.go`, `pkg/crypto/keyring.go`, test, `internal/config/config.go`.

- [ ] Định dạng `enc:v1:<keyID>:<base64>`; hàm `EncryptWithAAD` và `DecryptWithAAD`
- [ ] Key ring: `WEBHOOK_ENCRYPTION_KEYS=id1:<khóa>,id2:<khóa>` cùng `WEBHOOK_ENCRYPTION_ACTIVE_KEY_ID`; parse và validate
- [ ] Bỏ khóa mặc định `1234...` khỏi `envDefault`; `Validate` yêu cầu có khóa ở mọi môi trường trừ test
- [ ] Test: sai AAD, sai keyID, nhiều khóa trong ring, ciphertext bị sửa

**Xong khi:** package thuần, test xanh, chưa nối vào service.

### R2. Áp dụng vào service và worker (ưu tiên cao)

**File:** `internal/service/webhook_service.go`, `internal/worker/processor.go`, test.

- [ ] Tạo hoặc sửa secret: mã hóa bằng khóa active, AAD là id webhook
- [ ] Worker: giá trị có tiền tố `enc:` mà giải mã lỗi thì `SkipRetry` và ghi delivery thất bại kèm lý do rõ ràng, không bao giờ dùng ciphertext làm secret
- [ ] Giá trị không có tiền tố (dữ liệu cũ) chỉ được chấp nhận khi `WEBHOOK_ALLOW_LEGACY_PLAINTEXT=true`
- [ ] Xóa hàm `decryptSecretWithFallback`
- [ ] Test các nhánh: khóa đúng, khóa sai, dữ liệu cũ khi cờ bật và khi cờ tắt

**Xong khi:** không còn đường nào ký bằng ciphertext.

### R3. Lệnh mã hóa dữ liệu cũ

**File:** `cmd/tools/encrypt-webhook-secrets/main.go`, `Makefile`, test integration, `docs/runbooks/webhook-secrets-migration.md`.

- [ ] Mã hóa các secret plaintext theo lô, idempotent (bỏ qua dòng đã có `enc:`), cờ `--dry-run`, in báo cáo số dòng
- [ ] Test integration với dữ liệu hỗn hợp
- [ ] Runbook: deploy bản mới, chạy lệnh, rồi tắt `WEBHOOK_ALLOW_LEGACY_PLAINTEXT`

**Xong khi:** chạy lại lệnh lần hai không đổi gì.

### R4. Xoay khóa

**File:** `cmd/tools/encrypt-webhook-secrets/main.go`, `docs/runbooks/webhook-key-rotation.md`, test.

- [ ] Cờ `--rotate-to <keyID>` re-encrypt các dòng đang dùng khóa cũ, yêu cầu ring có cả khóa cũ và mới
- [ ] Test: sau khi rotate, giải mã được bằng khóa mới; ring chỉ còn khóa cũ thì lỗi rõ ràng
- [ ] Runbook các bước xoay khóa và thu hồi khóa cũ

**Xong khi:** test xanh, runbook đầy đủ.

## Giai đoạn S: RBAC và quản trị tối thiểu

### S1. ADR phạm vi admin và auditor

**File:** `docs/adr/0023-admin-auditor-scope.md`, cập nhật `docs/adr/0021-account-lockout-and-advanced-rbac.md`.

- [ ] Admin được làm gì (đổi role, mở khóa, xem audit) và có truy cập dữ liệu task của người khác hay không (khuyến nghị: không)
- [ ] Auditor chỉ đọc: audit log và metrics hệ thống
- [ ] Cách tạo admin đầu tiên (CLI, không qua API công khai) và quy tắc không hạ admin cuối cùng
- [ ] Ma trận quyền ánh xạ tới endpoint; mỗi hằng số quyền không dùng phải được gán endpoint hoặc bị xóa

**Xong khi:** chỉ có tài liệu.

### S2. Tạo admin và đổi role

**File:** `cmd/tools/create-admin/main.go`, `internal/service/admin_service.go`, `internal/transport/http/handler/admin.go`, `router.go`, test.

- [ ] CLI nâng một tài khoản đã có lên admin theo email (không nhận mật khẩu qua đối số dòng lệnh)
- [ ] `PATCH /api/v1/admin/users/:id/role`: chỉ admin, không hạ admin cuối cùng, thu hồi refresh token khi đổi role
- [ ] Test ma trận role nhân với endpoint

**Xong khi:** test xanh.

### S3. Mở khóa tài khoản

**Điều kiện:** sau S2 và Q1.

**File:** `queries/users.sql`, `internal/service/admin_service.go`, handler, router, test.

- [ ] `POST /api/v1/admin/users/:id/unlock`: admin reset bộ đếm và `locked_until`
- [ ] Test: user bị khóa, admin mở khóa, đăng nhập lại được; user thường gọi nhận 403

**Xong khi:** test xanh.

### S4. Audit log

**File:** `migrations/000013_audit_logs.up.sql` và `.down.sql`, `queries/audit_logs.sql`, `internal/service/admin_service.go`, handler, test.

- [ ] Bảng `audit_logs` (actor, action, target, metadata, created\_at)
- [ ] Ghi log cho đổi role và mở khóa
- [ ] `GET /api/v1/admin/audit/events` cho admin và auditor, phân trang keyset
- [ ] Test: auditor đọc được, không thực hiện được hành động ghi

**Xong khi:** test integration xanh.

### S5. Hoàn thiện tài liệu và kiểm tra quyền

**File:** `internal/transport/http/router_test.go`, swagger, README, ADR-0021.

- [ ] Test router xác nhận mọi route `/admin` yêu cầu đúng quyền
- [ ] Swagger cho endpoint admin; README có bảng role

**Xong khi:** mô tả trong README khớp hành vi thật.

## Giai đoạn T: Dọn dẹp và nhất quán

### T1. Xóa mã không dùng

**File:** `internal/service/webhook_service.go`, mock liên quan, test.

- [ ] Xóa `DispatchEvent` (không còn nơi gọi) khỏi interface, implementation và mock
- [ ] Chạy golangci-lint bật `unused`, chỉ xóa phần chắc chắn không dùng

**Xong khi:** lint và test xanh, hành vi không đổi.

### T2. Bỏ nhánh không-transaction ở project\_service

**File:** `internal/service/project_service.go`, `internal/service/testutil_store_test.go` (fake Store), test.

- [ ] Tạo fake `repository.Store` trong test để chạy được `WithTx` trên mock
- [ ] `RemoveMember` và `UpdateMemberRole`: đưa bước kiểm tra membership vào trong transaction; bỏ nhánh `else` không dùng transaction
- [ ] Test rollback vẫn xanh

**Xong khi:** không còn nhánh `s.repo.(repository.Store)` có `else` trong hai hàm trên.

### T3. Bỏ nhánh fallback ghi outbox ở các service còn lại

**File:** `internal/service/task_service.go`, `comment_service.go`, `attachment_service.go`, test.

- [ ] Xóa các nhánh không-transaction có `_, _ = ...CreateOutboxEvent` (lỗi bị nuốt); dùng fake Store ở T2 cho test
- [ ] Test: lỗi ghi outbox làm hỏng cả transaction nghiệp vụ

**Xong khi:** mọi đường ghi dữ liệu kèm sự kiện đều nằm trong một transaction.

### T4. Smoke Bash ngang PowerShell

**File:** `scripts/smoke-prod.sh`, `docs/smoke-test.md`.

- [ ] Bước 7 và 8 của bản Bash tự xác minh: giao webhook thật qua `scripts/webhook-receiver`, replay bị từ chối, nguồn B không bị 429
- [ ] Hai script dùng cùng tiêu chí PASS/FAIL

**Xong khi:** người dùng chạy được cả hai và ra kết quả tương đương.

## Giai đoạn U: Bằng chứng và phát hành v1.2.1

### U1. Smoke test mở rộng

**File:** `scripts/smoke-prod.sh`, `scripts/smoke-prod.ps1`, `docs/smoke-test.md`.

- [ ] Bước khóa tài khoản: sai mật khẩu nhiều lần, kiểm tra phản hồi theo ADR-0022, sau đó hết hạn hoặc admin mở khóa
- [ ] Bước hai replica: chạy 2 instance API (`--scale api=2` hoặc tương đương), phát sinh một sự kiện, webhook nhận đúng một lần
- [ ] Bước kiểm tra secret trong DB bắt đầu bằng `enc:v1:`
- [ ] Người dùng chạy và dán kết quả (ngày, commit, đầu ra); giữ lịch sử cũ

**Xong khi:** kết quả thật được ghi theo commit candidate.

### U2. CI

**File:** `.github/workflows/ci.yml`.

- [ ] Job integration chạy `-race` cho các test outbox và migration 000012, 000013 (up và down)
- [ ] Giữ ngưỡng coverage hiện có; govulncheck, trivy, gitleaks không đổi

**Xong khi:** CI xanh trên nhánh.

### U3. Chuẩn bị phát hành

**File:** `CHANGELOG.md`, `README.md`, `docs/RELEASE_NOTES_v1.2.1.md`, `docs/archive/`.

- [ ] Quyết định: không tag v1.2.0 vì chứa lỗi chưa sửa; ghi chú trong CHANGELOG rằng v1.2.0 chưa từng được phát hành và nội dung được gộp vào v1.2.1
- [ ] Release notes v1.2.1: Fixed (outbox giao trùng, bộ đếm khóa tài khoản, giải mã secret), Added (key ring, công cụ mã hóa dữ liệu cũ, endpoint admin, audit log), Security
- [ ] README cập nhật tag mẫu và các biến env mới
- [ ] Chuyển tài liệu kế hoạch này vào `docs/archive/` khi xong
- [ ] Agent chỉ chuẩn bị; người dùng tự tag `v1.2.1` sau khi CI xanh và smoke test đạt, rồi tạo GitHub Release

**Xong khi:** README khớp với code, không có tag cũ nào bị dời.

## Định nghĩa hoàn thành v1.2.1

- [ ] Hai relay chạy song song không giao trùng, có test integration và smoke hai replica
- [ ] Bộ đếm khóa tài khoản hoạt động đúng như ADR, phản hồi đăng nhập không lộ email
- [ ] Không còn đường ký webhook bằng ciphertext; secret cũ đã được mã hóa; có runbook xoay khóa
- [ ] Mọi quyền trong `RolePermissions` có endpoint thật hoặc đã bị xóa; admin tạo và mở khóa được, auditor đọc được audit log
- [ ] Smoke test Bash và PowerShell tương đương, có kết quả thật cho bản này
- [ ] CI xanh trên tag `v1.2.1`, image `1.2.1` có trên GHCR

## Ngoài phạm vi (để sau)

- Tích hợp KMS cho khóa mã hóa
- Xác thực hai lớp (MFA) và CAPTCHA khi đăng nhập
- Redis có tính sẵn sàng cao và tách worker thành tiến trình riêng
- Giới hạn tốc độ giao webhook theo từng đích
