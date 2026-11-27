# Kế hoạch phát triển v1.2.0: Bảo mật Webhook, Outbox Pattern & RBAC Nâng cao

Kế hoạch thực hiện 3 hạng mục kiến trúc trọng tâm sau bản phát hành v1.1.1:
1. **Mã hóa secret webhook (AES-256-GCM)** và tách secret ra khỏi message queue payload.
2. **Transactional Outbox Pattern** cho sự kiện webhook nhằm loại bỏ sự cố dual-write và bảo đảm at-least-once delivery.
3. **Account Lockout & RBAC nâng cao** nhằm chống tấn công brute-force phân tán theo từng tài khoản và phân quyền kiểm toán độc lập.

---

## 📌 Quy ước chung khi thực hiện task

- **Giới hạn phạm vi**: Mỗi commit sửa đổi tối đa khoảng 5 - 8 files. Nếu vượt quá, dừng lại và đề xuất tách task nhỏ hơn.
- **Không sửa file sinh tự động**: Tuyệt đối không sửa trực tiếp `internal/repository/*.sql.go` hay `docs/swagger/*`. Thay vào đó, sửa file nguồn (`queries/*.sql`, doc annotations) rồi chạy `make sqlc` / `make swagger`.
- **An toàn cấu hình**: Nếu thêm biến môi trường mới, phải cập nhật đồng thời:
  1. `internal/config/config.go`
  2. `.env.example`
  3. `docker-compose.prod.yml`
  4. Bảng biến môi trường trong `README.md`
- **Kiểm thử bắt buộc trước khi commit**:
  - `go test -short -race ./...` pass 100%.
  - `golangci-lint run` với 0 warnings/errors.
  - Task có can thiệp SQL/database phải chạy thêm `go test -tags=integration ./...`.
- **Quy tắc Git**: Commit theo chuẩn Conventional Commits (`feat:`, `fix:`, `chore:`, `refactor:`, `docs:`) kèm `--no-verify`. Không dời hay xóa các tag đã phát hành.
- **Cập nhật tiến độ**: Cuối mỗi task, đánh dấu `[x]` vào checklist tương ứng trong file kế hoạch này.

---

## 🔗 Thứ tự và phụ thuộc giữa các giai đoạn

- **Giai đoạn W (Mã hóa Webhook)**: Làm đầu tiên (`W1 -> W2 -> W3`). Độc lập và bảo vệ dữ liệu bí mật sớm nhất.
- **Giai đoạn O (Transactional Outbox)**: Làm sau giai đoạn W (`O1 -> O2 -> O3 -> O4`). Phụ thuộc vào việc webhook model và payload đã được chuẩn hóa.
- **Giai đoạn A (Account Lockout & RBAC)**: Làm sau giai đoạn O (`A1 -> A2 -> A3 -> A4`). Độc lập về mặt dữ liệu xác thực người dùng.
- **Giai đoạn R (Phát hành v1.2.0)**: Làm cuối cùng (`R1`).

---

## Giai đoạn W: Bảo mật Secret Webhook (AES-256-GCM)

Mục tiêu: Không còn lưu secret dưới dạng plaintext trong PostgreSQL và Redis.

### W1. Xây dựng thư viện mã hóa đối xứng chuẩn (`pkg/crypto`)

**Mục tiêu:** Cung cấp mô-đun mã hóa/giải mã chuẩn mật mã học AES-256-GCM không phụ thuộc thư viện thứ ba.

**File:** `pkg/crypto/aes_gcm.go`, `pkg/crypto/aes_gcm_test.go`.

- [x] Triển khai `Encrypt(plaintext []byte, key []byte) (ciphertextBase64 string, error)`: sử dụng AES-256-GCM với nonce ngẫu nhiên 12 bytes sinh từ `crypto/rand`, đóng gói `nonce + ciphertext + tag` và encode Base64.
- [x] Triển khai `Decrypt(ciphertextBase64 string, key []byte) ([]byte, error)`: decode Base64, trích xuất nonce 12 bytes và giải mã kèm xác thực tag toàn vẹn.
- [x] Định nghĩa các lỗi sentinel: `ErrInvalidKeySize`, `ErrCiphertextTooShort`, `ErrDecryptionFailed`.
- [x] Unit tests: kiểm thử round-trip, test vectors, tính ngẫu nhiên của nonce (mỗi lần mã hóa cùng plaintext ra kết quả khác nhau), và từ chối ciphertext bị giả mạo.

**Xong khi:** `go test ./pkg/crypto/...` pass 100%, coverage đạt trên 90%.

---

### W2. Migration DB & Tích hợp mã hóa vào Webhook Service

**Mục tiêu:** Secret được mã hóa trước khi ghi DB; hỗ trợ tương thích ngược không gián đoạn.

**File:** `internal/config/config.go`, `.env.example`, `docker-compose.prod.yml`, `README.md`, `internal/service/webhook_service.go`, `internal/service/webhook_service_test.go`.

- [ ] Thêm `WEBHOOK_ENCRYPTION_KEY` (khóa 32-byte hex/base64) vào `internal/config/config.go` kèm validation nghiêm ngặt trong staging/production.
- [ ] Cập nhật `.env.example`, `docker-compose.prod.yml`, và tài liệu `README.md`.
- [ ] Tại `CreateWebhook`: mã hóa secret bằng `pkg/crypto` trước khi lưu vào DB thông qua `queries.CreateWebhook`.
- [ ] Cung cấp cơ chế giải mã hỗ trợ fallback: nếu secret trong DB là plaintext cũ (chưa được mã hóa), giải mã trả về plaintext để duy trì tương thích trong quá trình rolling update.
- [ ] Viết unit & integration tests xác thực dữ liệu lưu trong DB là ciphertext base64.

**Xong khi:** Test service và integration pass xanh; secret lưu trong DB là ciphertext.

---

### W3. Tách Secret khỏi Payload Asynq & Tải Secret tại Worker

**Mục tiêu:** Payload Asynq/Redis chỉ mang ID, loại bỏ rủi ro rò rỉ secret qua hàng đợi.

**File:** `internal/worker/task.go`, `internal/worker/processor.go`, `internal/worker/processor_test.go`, `internal/service/webhook_service.go`.

- [ ] Sửa `SendWebhookPayload`: xóa hoàn toàn trường `Secret string`, chỉ truyền `WebhookID uuid.UUID`.
- [ ] Sửa `webhookService.DispatchEvent`: không còn gán `Secret` vào payload khi enqueue task.
- [ ] Sửa `processor.ProcessSendWebhook`: dùng `payload.WebhookID` truy vấn thông tin webhook từ DB, giải mã secret bằng master key và thực hiện `webhook.SignV2`.
- [ ] Nếu webhook không còn tồn tại hoặc đã bị xóa: bỏ qua với `asynq.SkipRetry` để tránh retry vô ích.
- [ ] Cập nhật toàn bộ worker tests và test fixtures.

**Xong khi:** Payload trong Redis không chứa secret, worker giải mã và ký chữ ký Webhook V2 hợp lệ, tests pass với `-race`.

---

## Giai đoạn O: Transactional Outbox Pattern cho Sự kiện Webhook

Mục tiêu: Đảm bảo tính nguyên tử giữa dữ liệu nghiệp vụ và sự kiện; loại bỏ dual-write problem.

### O1. Thiết kế kiến trúc Outbox (ADR-0020)

**Mục tiêu:** Thống nhất mô hình dữ liệu outbox, cơ chế batch polling và chu kỳ dọn dẹp.

**File:** `docs/adr/0020-transactional-outbox-pattern.md`.

- [ ] Đặc tả schema bảng `outbox_events`: `id`, `event_type`, `aggregate_type`, `aggregate_id`, `actor_id`, `project_id`, `payload`, `status` (`pending`, `published`, `failed`), `retry_count`, `last_error`, `created_at`, `processed_at`.
- [ ] Xác lập partial index phục vụ truy vấn hàng đợi: `idx_outbox_events_pending ON outbox_events(created_at) WHERE status = 'pending'`.
- [ ] Chọn chiến lược relay: Polling định kỳ sử dụng `SELECT ... FOR UPDATE SKIP LOCKED LIMIT $1` để an toàn khi chạy nhiều worker song song.
- [ ] Quy định chính sách lưu trữ (retention): cron job dọn dẹp các sự kiện đã `published` sau 7 ngày.

**Xong khi:** Có file ADR-0020 hoàn chỉnh, không chạm vào code.

---

### O2. Migration Database & Sqlc Queries cho Outbox

**Mục tiêu:** Khởi tạo bảng outbox trong PostgreSQL và sinh repository methods an toàn kiểu dữ liệu.

**File:** `migrations/000010_create_outbox_events_table.up.sql`, `migrations/000010_create_outbox_events_table.down.sql`, `queries/outbox.sql`, `internal/repository/outbox.sql.go`.

- [ ] Viết migration tạo bảng `outbox_events` kèm partial index và constraints.
- [ ] Viết các câu lệnh SQL trong `queries/outbox.sql`: `CreateOutboxEvent`, `FetchPendingOutboxEvents` (`FOR UPDATE SKIP LOCKED`), `MarkOutboxEventsPublished`, `MarkOutboxEventFailed`, `DeleteOldProcessedOutboxEvents`.
- [ ] Chạy `make sqlc` sinh mã nguồn Go.
- [ ] Kiểm thử migration up/down với script kiểm tra migration.

**Xong khi:** Migration và sqlc compile sạch, test migration up/down thành công.

---

### O3. Ghi nhận Outbox trong cùng Transaction nghiệp vụ

**Mục tiêu:** Giao dịch DB nghiệp vụ và sự kiện outbox luôn cam kết hoặc hủy bỏ cùng nhau.

**File:** `internal/service/task_service.go`, `internal/service/comment_service.go`, `internal/service/attachment_service.go`, tests liên quan.

- [ ] Tích hợp ghi `CreateOutboxEvent` vào cùng database transaction (`store.ExecTx`) trong:
  - `TaskService`: `CreateTask`, `UpdateTask`, `DeleteTask`, `UpdateTaskStatus`.
  - `CommentService`: `CreateComment`.
  - `AttachmentService`: `UploadAttachment`.
- [ ] Loại bỏ việc gọi trực tiếp `distributor.DistributeSendWebhook` trong luồng xử lý HTTP request.
- [ ] Unit tests cho service layers xác nhận khi transaction rollback thì không có sự kiện outbox nào tồn tại.

**Xong khi:** Unit tests pass xanh, kiểm thử chứng minh dual-write được loại bỏ hoàn toàn.

---

### O4. Xây dựng Outbox Relay Worker & Integration Test

**Mục tiêu:** Đọc outbox events định kỳ, dispatch webhook và đánh dấu đã xử lý.

**File:** `internal/worker/outbox_relay.go`, `internal/worker/processor.go`, `cmd/api/main.go`, `internal/service/outbox_integration_test.go`.

- [ ] Xây dựng Outbox Relay loop: định kỳ (mỗi 500ms - 1s) lấy batch pending events bằng `FetchPendingOutboxEvents`.
- [ ] Tìm danh sách webhook hợp lệ cho từng event và enqueue task `task:send_webhook` (hoặc dispatch) vào queue Asynq.
- [ ] Cập nhật trạng thái các event thành `published` trong một database transaction.
- [ ] Thêm background job dọn dẹp các outbox events cũ đã hoàn tất (`published` quá 7 ngày).
- [ ] Viết Live Integration test với Testcontainers PostgreSQL: tạo task qua service $\to$ commit DB $\to$ relay quét và dispatch thành công.

**Xong khi:** Test integration chạy xanh với `-race`, không có hiện tượng race condition khi chạy nhiều relay workers.

---

## Giai đoạn A: Account Lockout & RBAC Nâng cao

Mục tiêu: Chống tấn công brute-force phân tán theo từng tài khoản và phân quyền kiểm toán độc lập.

### A1. Thiết kế Chính sách Khóa Tài khoản & RBAC Matrix (ADR-0021)

**Mục tiêu:** Xác lập đặc tả kỹ thuật cho Account Lockout và phân quyền chi tiết.

**File:** `docs/adr/0021-account-lockout-and-advanced-rbac.md`.

- [ ] Đặc tả quy tắc Account Lockout:
  - Ngưỡng vi phạm: 5 lần đăng nhập sai mật khẩu liên tiếp $\to$ khóa tài khoản trong 15 phút.
  - Reset vi phạm: Đăng nhập thành công $\to$ reset counter về 0 và xóa `locked_until`.
  - Cơ chế tự phục hồi: Sau 15 phút, tài khoản tự động được phép thử lại mà không cần can thiệp admin.
  - Chống timing attack / user enumeration: Khi tài khoản đang bị khóa hoặc không tồn tại, hệ thống luôn thực hiện dummy hash comparison trước khi trả lỗi.
- [ ] Đặc tả RBAC nâng cao:
  - Vai trò hệ thống: `admin`, `auditor`, `user`.
  - Phân quyền theo capabilities/scopes: `users:read`, `users:manage`, `audit:read`, `system:metrics`.
  - Vai trò `auditor`: được quyền đọc metrics, audit logs, webhook deliveries nhưng bị chặn toàn bộ thao tác ghi/sửa/xóa.

**Xong khi:** Có file ADR-0021 hoàn chỉnh, không chạm vào code.

---

### A2. Migration DB & Sqlc Queries cho Account Lockout

**Mục tiêu:** Bổ sung các cột theo dõi đăng nhập vào bảng `users` và sinh queries sqlc.

**File:** `migrations/000011_add_account_lockout_to_users.up.sql`, `migrations/000011_add_account_lockout_to_users.down.sql`, `queries/users.sql`, `internal/repository/users.sql.go`.

- [ ] Migration thêm các cột vào `users`:
  - `failed_login_attempts INT NOT NULL DEFAULT 0`
  - `locked_until TIMESTAMPTZ NULL`
  - `last_failed_login_at TIMESTAMPTZ NULL`
- [ ] Viết queries trong `queries/users.sql`:
  - `RecordFailedLogin`: tăng `failed_login_attempts`, nếu $\ge 5$ thì set `locked_until = NOW() + INTERVAL '15 minutes'`.
  - `ResetFailedLogin`: đặt lại `failed_login_attempts = 0` và `locked_until = NULL`.
- [ ] Chạy `make sqlc` sinh repository code.

**Xong khi:** Migration và sqlc compile sạch, test migration up/down thành công.

---

### A3. Áp dụng Account Lockout vào AuthService & HTTP 423

**Mục tiêu:** Kích hoạt logic khóa tài khoản khi đăng nhập và trả mã lỗi RFC 9457 chuẩn.

**File:** `internal/domain/errors.go`, `internal/service/auth_service.go`, `internal/transport/http/handler/auth.go`, `internal/transport/http/middleware/error_handler.go`, `internal/service/auth_service_test.go`.

- [ ] Định nghĩa lỗi `ErrAccountLocked` tương ứng HTTP 423 `Locked` (`ACCOUNT_LOCKED`) trong RFC 9457 Problem Details.
- [ ] Trong `authService.Login`:
  - Nếu `user.LockedUntil > time.Now()`: chạy dummy hash comparison chống timing attack, trả về `ErrAccountLocked`.
  - Nếu sai mật khẩu: gọi `RecordFailedLogin`, trả về `ErrInvalidCredentials`.
  - Nếu đăng nhập thành công: gọi `ResetFailedLogin` và cấp phát token pair.
- [ ] Unit tests bao phủ toàn diện: đúng sau 4 lần sai, bị khóa ở lần 5, thử đăng nhập khi đang bị khóa nhận HTTP 423, và đăng nhập bình thường sau khi hết 15 phút.

**Xong khi:** Toàn bộ test auth pass xanh, không rò rỉ timing attack.

---

### A4. Triển khai RBAC Nâng cao & Permission Middleware

**Mục tiêu:** Mở rộng vai trò `auditor` và cơ chế kiểm soát truy cập dựa trên permission.

**File:** `internal/domain/user.go`, `internal/transport/http/middleware/rbac.go`, `internal/transport/http/middleware/rbac_test.go`, `internal/transport/http/router.go`.

- [ ] Bổ sung vai trò `auditor` vào enum vai trò hệ thống (`domain.RoleAuditor`).
- [ ] Tạo middleware `RequireAnyRole(roles ...string)` và `RequirePermission(perm string)`.
- [ ] Bảo vệ các routes quản trị: cho phép `auditor` xem `/metrics`, deliveries, logs; cấm sửa đổi tài nguyên nghiệp vụ.
- [ ] Unit tests kiểm thử ma trận phân quyền mới.

**Xong khi:** Linter 0 issue, test pass xanh, các quyền cũ của `admin` và `user` hoạt động nguyên vẹn.

---

## Giai đoạn R: Chuẩn bị phát hành v1.2.0

### R1. Phát hành v1.2.0

**File:** `CHANGELOG.md`, `README.md`, `docs/RELEASE_NOTES_v1.2.0.md`.

- [ ] Cập nhật `CHANGELOG.md` cho bản `[1.2.0]`: Fixed, Added, Changed, Security.
- [ ] Cập nhật `README.md`: tài liệu hóa các tính năng mới (Outbox, AES-GCM, Account Lockout, Auditor role) và liên kết ADR-0020, ADR-0021.
- [ ] Tạo file release notes `docs/RELEASE_NOTES_v1.2.0.md`.
- [ ] Chuyển file kế hoạch này vào `docs/archive/ROADMAP_v1.2.0.md`.

**Xong khi:** Tài liệu khớp hoàn toàn với code, CI xanh trên main.

---

## 🎯 Định nghĩa hoàn thành v1.2.0 (Definition of Done)

- [ ] Secret webhook được mã hóa AES-256-GCM trong PostgreSQL; không xuất hiện plaintext trong database và Redis payload.
- [ ] Sự kiện webhook được ghi nhận trong cùng transaction nghiệp vụ thông qua Outbox table; Relay worker vận hành ổn định và không phát sinh dual-write / ghost events.
- [ ] Tài khoản bị khóa 15 phút sau 5 lần thử sai liên tiếp (HTTP 423 Locked) có bảo vệ timing attack.
- [ ] Vai trò `auditor` được phân quyền độc lập, chỉ xem tài nguyên kiểm toán.
- [ ] Test coverage $\ge 65\%$ trên service layer, 0 issue trên `golangci-lint`, 0 lỗ hổng trên `govulncheck`.
