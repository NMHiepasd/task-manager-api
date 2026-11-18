# Kế hoạch cải thiện v1.1: Task Manager REST API

Nguồn: kết quả review mã nguồn sau v1.0.0 (webhook, phân quyền comment/attachment, test). Mỗi task đủ nhỏ để một phiên agent hoàn thành, không kèm thời gian dự kiến.

## Quy ước chung cho mọi prompt

- Không đọc hay sửa file sinh tự động (`internal/repository/*.sql.go`, `docs/swagger/*`): sửa nguồn rồi chạy `make sqlc` / `make swagger`
- Viết test tái hiện trước, sửa sau (đỏ rồi xanh)
- Chạm tối đa khoảng 8 file; vượt thì dừng, báo cáo và đề xuất tách task
- Thêm biến env thì cập nhật cùng lúc `.env.example`, `docker-compose.prod.yml` và bảng env trong README
- Trước khi commit: `make lint` và `go test -race -short ./...`; task có SQL thì thêm `make test-integration`
- Việc agent không chạy được (docker compose thật) phải ghi rõ trong báo cáo để người dùng tự kiểm tra
- Không dời hay xóa tag đã phát hành; bản sửa dùng tag mới (`v1.1.0`)
- Cuối task: tick checklist trong tài liệu này, commit theo Conventional Commits

Mẫu prompt:

```
Đọc docs/improvement-plan.md. Chỉ thực hiện Task A1, không làm task khác.
Chỉ chạm các file liệt kê trong task. Viết test tái hiện trước khi sửa.
Chạy lệnh kiểm tra, tick checklist, commit rồi báo cáo: file đã đổi, kết quả test, việc còn tồn.
```

## Thứ tự và phụ thuộc

- Giai đoạn A, B, C là ưu tiên cao
- Trong B: B1 → B2 → B3
- Trong C: C1 → C2 → C3..C6 → C7
- E1 làm sau C3
- Các giai đoạn còn lại độc lập

## Giai đoạn A: Sửa phạm vi và vòng đời webhook (ưu tiên cao)

### A1. Sửa query chọn webhook khi dispatch

**Mục tiêu:** webhook gắn project chỉ nhận sự kiện của đúng project đó, và chỉ khi chủ webhook còn là owner/editor.

**File:** `queries/webhooks.sql`, `internal/service/webhook_service.go`, file test integration mới.

- [x] Test integration (build tag `integration`): webhook của editor đã bị xóa khỏi project không nhận sự kiện; webhook project A không nhận sự kiện của project B; webhook không gắn project vẫn nhận sự kiện của chủ
- [x] Viết lại `ListActiveWebhooksForEvent`: nhánh `user_id` chỉ áp dụng cho webhook có `project_id IS NULL`; nhánh project JOIN `projects`/`project_members` để kiểm tra chủ webhook là owner hoặc editor
- [x] `make sqlc`, chạy lại toàn bộ test

**Xong khi:** 3 test mới xanh, không test cũ nào hỏng.

### A2. Vô hiệu webhook khi đổi quyền thành viên

**Mục tiêu:** gỡ member hoặc hạ xuống viewer thì webhook project của người đó ngừng hoạt động.

**File:** `queries/webhooks.sql`, `internal/service/project_service.go`, test service và integration.

- [x] Thêm query `DeactivateProjectWebhooksByUser(project_id, user_id)`
- [x] Gọi trong `RemoveMember` và trong `UpdateMemberRole` khi role mới là viewer (cùng transaction với thay đổi member nếu có `WithTx`)
- [x] Test: sau khi gỡ/hạ quyền, `is_active = false`; webhook cá nhân của user không bị ảnh hưởng

**Xong khi:** test xanh, hành vi cache cũ giữ nguyên.

### A3. Sentinel error cho SSRF và cấu hình retry

**Mục tiêu:** bỏ so khớp chuỗi lỗi, giới hạn số lần retry.

**File:** `pkg/webhook/client.go`, `pkg/webhook/security.go`, `internal/worker/processor.go`, `internal/worker/distributor.go`, `internal/config/config.go`, test.

- [x] Thêm `ErrBlockedIP`; `Dialer.Control` trả lỗi bọc bằng `%w`; processor dùng `errors.Is` thay cho `strings.Contains`
- [x] Task webhook dùng `asynq.MaxRetry` (env `WEBHOOK_MAX_RETRY`, mặc định 6) và `asynq.Timeout(15s)`
- [x] Test: IP bị chặn thì `SkipRetry`; lỗi mạng thường thì retry

**Xong khi:** không còn `strings.Contains(err.Error()` trong processor.

### A4. Chính sách secret của webhook

**Mục tiêu:** secret đủ mạnh và không bị lộ lại sau khi tạo.

**File:** `internal/service/webhook_service.go`, `internal/transport/http/handler/webhook.go`, test, README.

- [x] Secret do người dùng tự đặt phải từ 32 ký tự; để trống thì server sinh ngẫu nhiên (24 byte trở lên)
- [x] Chỉ trả `secret` ở response tạo mới; get/list/update không trả
- [x] Test các trường hợp trên; chạy `make swagger`

**Xong khi:** test xanh, swagger khớp.

## Giai đoạn B: Chữ ký webhook phiên bản 2 (chống replay)

### B1. Hàm ký và xác minh có timestamp

**Mục tiêu:** thư viện thuần, chưa nối vào luồng gửi.

**File:** `pkg/webhook/signature.go`, `pkg/webhook/signature_test.go`.

- [x] `SignV2(secret, ts, id string, body []byte) string`: HMAC-SHA256 trên chuỗi `ts.id.body`
- [x] `VerifyV2(secret, ts, id string, body []byte, sigHeader string, now time.Time, tolerance time.Duration) error`: so sánh constant-time, trả `ErrSignatureMismatch` hoặc `ErrTimestampExpired`
- [x] Test: đúng, body bị sửa, id bị đổi, timestamp quá hạn, timestamp ở tương lai, chữ ký sai định dạng hex
- [x] Giữ hàm cũ chưa xóa

**Xong khi:** test xanh, hàm có comment mô tả định dạng.

### B2. Gắn vào luồng gửi và retry

**Mục tiêu:** mỗi sự kiện có ID ổn định, mỗi lần gửi có timestamp mới.

**File:** `internal/worker/task.go`, `internal/worker/distributor.go`, `internal/worker/processor.go`, `internal/service/webhook_service.go`, test.

- [x] `SendWebhookPayload` thêm `DeliveryID` (UUID sinh lúc dispatch, giữ nguyên qua retry); envelope JSON thêm trường `id`
- [x] Processor mỗi lần gửi tạo timestamp mới và đặt header `X-Webhook-Id`, `X-Webhook-Timestamp`, `X-Webhook-Signature: v1=<hex>`; bỏ header ký kiểu cũ
- [x] Áp dụng cả cho `TriggerTestPing`
- [x] Test: bên nhận verify được bằng `VerifyV2`; lần retry có timestamp mới nhưng cùng id

**Xong khi:** test xanh; ghi rõ trong báo cáo đây là thay đổi phá vỡ tương thích.

### B3. Tài liệu cho bên nhận webhook

**Mục tiêu:** người tích hợp biết cách xác minh đúng.

**File:** `docs/webhooks.md`, `README.md`, `CHANGELOG.md`, mô tả swagger trong handler.

- [x] Mô tả header, thuật toán, tolerance khuyến nghị 5 phút, loại trùng theo `X-Webhook-Id`, cách xoay secret
- [x] Ví dụ verify bằng Go, Node, Python
- [x] Mục Unreleased trong CHANGELOG, ghi rõ thay đổi phá vỡ

**Xong khi:** chỉ có thay đổi tài liệu, README có link tới `docs/webhooks.md`.

## Giai đoạn C: Gom và siết phân quyền (ưu tiên cao)

### C1. ADR chính sách truy cập task

**Mục tiêu:** chốt luật trước khi viết code.

**File:** `docs/adr/0018-task-access-policy.md`.

- [x] Bảng quyền: vai trò (owner, editor, viewer, creator, assignee, người ngoài) nhân với hành động (đọc, comment, sửa, xóa, upload, kiểm duyệt comment, kiểm duyệt attachment)
- [x] Task thuộc project: chỉ membership có hiệu lực. Task cá nhân: creator/assignee
- [x] Chốt: viewer có được comment không; khi member rời project thì assignee bị gỡ (`assignee_id = NULL`)
- [x] Ghi lý do mỗi quyết định

**Xong khi:** chỉ có file ADR, không đụng code.

### C2. Package `internal/authz`

**Mục tiêu:** một nơi duy nhất quyết định quyền, chưa nối vào service.

**File:** `internal/authz/authz.go`, `internal/authz/authz_test.go`.

- [x] Kiểu `Action` (Read, Comment, Edit, Delete, Upload, ModerateComments, ModerateAttachments)
- [x] Hàm thuần `Can(subject, resource, action) bool` nhận dữ liệu đã nạp sẵn (task, project, role của member), không gọi DB
- [x] Test table-driven phủ toàn bộ ma trận trong ADR-0018

**Xong khi:** test phủ mọi ô của ma trận, package không import `gin` hay `repository`.

### C3. Nối comment\_service vào authz

**File:** `internal/service/comment_service.go`, `comment_service_test.go`.

- [x] Thay `verifyTaskAccess` và logic quyền xóa bằng `authz.Can`
- [x] Test mới: viewer theo đúng ADR, editor không kiểm duyệt được, owner kiểm duyệt được, creator đã rời project nhận 404, project xóa mềm nhận 404

**Xong khi:** test cũ và mới xanh.

### C4. Nối attachment\_service vào authz

**File:** `internal/service/attachment_service.go`, `attachment_service_test.go`.

- [x] Thay `verifyTaskReadAccess` / `verifyTaskWriteAccess`; xóa attachment dùng `ModerateAttachments`
- [x] Test: viewer là creator không xóa được attachment của người khác; creator đã rời project bị chặn cả đọc lẫn ghi

**Xong khi:** test xanh.

### C5. Nối task\_service: các đường đọc

**File:** `internal/service/task_service.go` (phần Get, List theo project, đọc tag), `task_service_test.go`.

- [x] Dùng `authz.Can(..., Read)`
- [x] Test: creator đã rời project không đọc được task của project

**Xong khi:** test xanh; không sửa các hàm ghi.

### C6. Nối task\_service: các đường ghi và dọn assignee

**File:** `internal/service/task_service.go` (Update, Delete, SetTags, assignee), `queries/tasks.sql`, `internal/service/project_service.go`, test.

- [x] Dùng `authz.Can` cho Edit/Delete
- [x] Query `UnassignTasksInProject(project_id, user_id)`; `RemoveMember` gọi theo ADR-0018
- [x] Test: member rời project thì task được giao cho họ về `assignee_id = NULL`

**Xong khi:** test xanh.

### C7. Bảng test quyền tích hợp

**Mục tiêu:** test sống bằng DB thật, đồng thời là tài liệu chính sách.

**File:** `internal/service/authz_matrix_integration_test.go` (build tag `integration`).

- [x] Dựng service thật bằng testcontainers
- [x] Bảng: vai trò (owner, editor, viewer, member bị xóa, creator không phải member, assignee không phải member, người ngoài) nhân với hành động nhân với trạng thái (bình thường, task xóa mềm, project xóa mềm)
- [x] Chỉ viết test; nếu phát hiện lỗi thì ghi thành task mới, không sửa lan

**Xong khi:** `make test-integration` xanh.

## Giai đoạn D: Attachment và storage

### D1. Presigned URL dùng được từ ngoài

**File:** `pkg/storage/storage.go`, `internal/config/config.go`, `deploy/caddy/Caddyfile`, `docker-compose.prod.yml`, test.

- [x] Env `S3_PUBLIC_ENDPOINT`; dùng client riêng chỉ để ký URL (không cần kết nối)
- [x] Caddy thêm route công khai tới MinIO, hoặc env `ATTACHMENT_PRESIGN_ENABLED=false` thì API chỉ stream
- [x] Test: URL ký ra có host công khai

**Xong khi:** test xanh; ghi vào báo cáo bước người dùng cần tự thử trên compose prod.

### D2. Download an toàn

**File:** `internal/transport/http/handler/attachment.go`, test.

- [x] Tạo `Content-Disposition` bằng `mime.FormatMediaType` (tham số filename) thay cho `Sprintf`
- [x] Thêm `X-Content-Type-Options: nosniff`
- [x] Test tên file chứa dấu nháy kép, xuống dòng, ký tự Unicode

**Xong khi:** test xanh.

### D3. Hạn mức attachment

**File:** `queries/task_attachments.sql`, `internal/service/attachment_service.go`, `internal/config/config.go`, test.

- [x] Env `MAX_ATTACHMENTS_PER_TASK` (mặc định 20) và `MAX_USER_STORAGE_MB` (mặc định 500)
- [x] Query đếm số file theo task và tổng dung lượng theo user
- [x] Vượt hạn mức trả lỗi Problem Details mã `ATTACHMENT_QUOTA_EXCEEDED`

**Xong khi:** test xanh, swagger cập nhật.

### D4. Ràng buộc SQL khi xóa

**File:** `queries/task_comments.sql`, `queries/task_attachments.sql`, `internal/service/comment_service.go`, `internal/service/attachment_service.go`, test integration.

- [x] Thêm `AND task_id = $2` vào hai câu lệnh xóa, cập nhật chỗ gọi
- [x] Test integration: xóa với task\_id sai không xóa được dòng nào

**Xong khi:** test xanh.

### D5. Xử lý object mồ côi

**File:** `internal/worker/task.go`, `internal/worker/processor.go`, `internal/worker/distributor.go`, `internal/service/attachment_service.go`, test.

- [x] Khi `storage.Delete` lỗi thì enqueue task `storage:delete_object` có retry thay vì bỏ qua
- [x] Test processor: retry khi lỗi tạm thời, bỏ qua khi object đã không tồn tại

**Xong khi:** test xanh.

## Giai đoạn E: Quyền riêng tư và hardening

### E1. Bỏ email khỏi dữ liệu chia sẻ

**Điều kiện:** sau C3.

**File:** `internal/service/comment_service.go`, payload webhook, swagger, test, README, CHANGELOG.

- [x] Bỏ trường `email` khỏi `CommentResponse` và khỏi payload webhook, giữ `author` và `user_id`
- [x] `make swagger`, cập nhật test và tài liệu

**Xong khi:** không còn email của người khác trong response/payload.

### E2. Validate config bổ sung

**File:** `internal/config/config.go`, `config_test.go`, `docker-compose.prod.yml`.

- [x] `APP_ENV` chỉ nhận development, staging, production, test
- [x] Production bắt buộc `GIN_MODE=release` và `REDIS_PASSWORD` không rỗng
- [x] `RequestTimeout` phải nhỏ hơn `InProgressLockTTL` (60s)
- [x] Compose dùng `${REDIS_PASSWORD:?...}`

**Xong khi:** test table-driven cho từng điều kiện xanh.

### E3. Ngách race của idempotency

**File:** `internal/transport/http/middleware/idempotency.go`, `queries/idempotency_keys.sql`, test.

- [x] Lỗi unique khi đặt lại key trả 409 `IDEMPOTENCY_IN_PROGRESS` thay vì 500
- [x] Xóa lock quá hạn có điều kiện theo `locked_at` thay vì xóa theo key
- [x] Test chạy song song với `-race`

**Xong khi:** test xanh, không có data race.

### E4. Ghim image bằng digest

**File:** `docker-compose.prod.yml`, `docker-compose.yml`, `.github/dependabot.yml`, README.

- [x] Ghim digest cho minio, postgres, redis, caddy, migrate
- [x] Dependabot bật ecosystem `docker` và `docker-compose`
- [x] Ghi cách cập nhật digest trong README

**Xong khi:** không còn tag trôi nổi như `latest`.

## Giai đoạn F: Bằng chứng và phát hành

### F1. Script smoke test

**File:** `scripts/smoke-prod.sh`, `docs/smoke-test.md`.

- [x] Agent viết script và hướng dẫn: compose up từ volume trống, `/readyz`, `/metrics` và `/swagger` trả 403, rate limit tách theo IP, đăng ký/đăng nhập, upload, presigned, webhook trỏ IP nội bộ bị từ chối, chữ ký webhook xác minh được
- [x] Người dùng tự chạy rồi dán kết quả (ngày, commit, đầu ra) vào `docs/smoke-test.md`

**Xong khi:** có kết quả thật được ghi lại.

### F2. Nâng cấp CI

**File:** `.github/workflows/ci.yml`.

- [x] Integration test chạy `./...` thay vì chỉ package repository
- [x] Xuất coverage, upload artifact, ngưỡng tối thiểu khoảng 65% cho tầng service
- [x] Thêm job gitleaks; govulncheck và trivy xuất báo cáo dạng artifact
- [x] Ghim phiên bản các action còn trôi

**Xong khi:** CI xanh trên nhánh.

### F3. Bằng chứng bảo mật

**File:** `docs/security.md`.

- [x] Người dùng chạy govulncheck, gosec, trivy, gitleaks; agent điền bảng kết quả (công cụ, phiên bản, ngày, số cảnh báo, ghi chú)
- [x] Cập nhật checklist OWASP API Top 10 theo thực tế

**Xong khi:** không còn câu chung chung kiểu kiểm tra định kỳ mà thiếu số liệu.

### F4. Dọn tài liệu và chuẩn bị phát hành

**File:** `docs/`, `README.md`, `CHANGELOG.md`.

- [x] Chuyển `docs/REMEDIATION_FEEDBACK_PLAN.md` và tài liệu kế hoạch này vào `docs/archive/` khi hoàn tất
- [x] README thêm mục webhook và chính sách quyền (link ADR-0018)
- [x] CHANGELOG cho v1.1.0
- [x] Agent chỉ chuẩn bị; người dùng tự tag `v1.1.0` sau khi CI xanh và tạo GitHub Release

**Xong khi:** README khớp với code và tài liệu.

## Định nghĩa hoàn thành v1.1.0

- [x] Giai đoạn A, B, C xong, `make test-integration` xanh gồm cả bảng test quyền
- [x] Compose prod chạy từ volume trống và smoke test đã ghi kết quả thật
- [x] CI xanh, báo cáo quét bảo mật đã lưu
- [x] Không còn email người khác trong response/payload, không còn tag image trôi nổi

## Ngoài phạm vi (để sau)

- Mã hóa secret webhook khi lưu trong DB và hàng đợi
- Outbox pattern cho sự kiện webhook
- RBAC nâng cao ngoài ba vai trò hiện có
