# Kế hoạch v1.1.1: Hoàn thiện sau review v1.1.0

Nguồn: review bản v1.1.0 (điểm 90/100). Mục tiêu: sửa lỗi cấu hình triển khai, bổ sung bằng chứng kiểm thử, siết tính nhất quán dữ liệu và quyền. Không kèm thời gian dự kiến.

## Quy ước chung (giữ như kế hoạch v1.1)

- Không đọc hay sửa file sinh tự động (`internal/repository/*.sql.go`, `docs/swagger/*`): sửa nguồn rồi chạy `make sqlc` / `make swagger`
- Viết test tái hiện trước, sửa sau (đỏ rồi xanh)
- Chạm tối đa khoảng 8 file; vượt thì dừng, báo cáo và đề xuất tách task
- Thêm biến env thì cập nhật cùng lúc `.env.example`, `docker-compose.prod.yml` và bảng env trong README
- Trước khi commit: `make lint` và `go test -race -short ./...`; task có SQL thì thêm `make test-integration`
- Việc agent không chạy được (docker compose thật, Caddy thật) phải ghi rõ trong báo cáo để người dùng tự kiểm tra
- Không dời hay xóa tag đã phát hành
- Cuối task: tick checklist trong tài liệu này, commit theo Conventional Commits

## Thứ tự và phụ thuộc

- G1 làm đầu tiên (nhanh, rủi ro triển khai cao nhất)
- G3a và G3b sau G2; G4 sau toàn bộ G
- H3a trước H3b
- Giai đoạn I độc lập; K làm cuối cùng

## Giai đoạn G: Triển khai và bằng chứng (ưu tiên cao)

### G1. Sửa tag image mặc định

**Mục tiêu:** cấu hình mặc định không còn kéo về image 1.0.0 đã lỗi thời.

**File:** `docker-compose.prod.yml`, `.env.example`, `README.md`, `scripts/smoke-prod.sh`.

- [x] Đổi `IMAGE_TAG:-1.0.0` thành bắt buộc `${IMAGE_TAG:?IMAGE_TAG is required}` ở cả `image` và build arg `VERSION`
- [x] `.env.example` và README ghi cách đặt `IMAGE_TAG=1.1.1`
- [x] Script smoke (hoặc CI) kiểm tra compose không còn chuỗi tag cũ `1.0.0`

**Xong khi:** `docker compose -f docker-compose.prod.yml config` báo lỗi rõ ràng khi thiếu `IMAGE_TAG`. Agent không chạy được compose thì ghi để người dùng kiểm tra.

### G2. Thu hẹp route MinIO ở Caddy

**Mục tiêu:** đường công khai tới MinIO chỉ phục vụ tải file.

**File:** `deploy/caddy/Caddyfile`, `docs/smoke-test.md`, `scripts/smoke-prod.sh`, `scripts/smoke-prod.ps1`.

- [x] Thêm matcher chỉ cho method GET và HEAD tới đường dẫn bucket; method khác trả 405 hoặc 403
- [x] Giữ nguyên khối chặn `/metrics`, `/swagger`, `/debug/pprof`
- [x] Thêm bước smoke: PUT và DELETE tới đường dẫn bucket qua Caddy bị từ chối, GET presigned vẫn tải được

**Xong khi:** người dùng chạy `caddy validate` và bước smoke mới cho kết quả đúng.

### G3a. Smoke: cách ly rate limit theo IP client

**Mục tiêu:** có bằng chứng cho lỗi nặng nhất ban đầu (mọi user dùng chung một bucket sau proxy).

**File:** `scripts/smoke-prod.sh`, `scripts/smoke-prod.ps1`, `docs/smoke-test.md`.

- [x] Gửi burst login từ nguồn A đến khi 429, ngay sau đó request từ nguồn B vẫn không bị 429
- [x] Spoof test: cùng một nguồn gửi header `X-Forwarded-For` khác nhau vẫn dùng chung bucket (header do client gửi không được tin)
- [x] Nếu script không giả lập được hai IP, ghi hướng dẫn thủ công chạy từ hai máy

**Xong khi:** script có bước mới, in rõ PASS hoặc FAIL.

### G3b. Smoke: giao webhook thật và verify chữ ký V2

**Mục tiêu:** chứng minh luồng giao webhook V2 chạy đầu cuối, không chỉ tính chữ ký cục bộ.

**File:** `scripts/webhook-receiver/` (chương trình Go hoặc Python nhỏ), `scripts/smoke-prod.sh`, `docs/smoke-test.md`, `internal/config/config.go` (chỉ nếu chọn phương án b).

- [x] Vì SSRF guard chặn IP nội bộ, chọn một phương án và ghi vào docs: (a) receiver ở URL công khai do người dùng cấp qua `SMOKE_WEBHOOK_URL`; (b) cờ `WEBHOOK_ALLOW_PRIVATE_HOSTS` chỉ cho môi trường test, `Validate` phải từ chối khi `APP_ENV=production` (kèm test)
- [x] Receiver verify bằng `pkg/webhook.VerifyV2` (hoặc ví dụ trong `docs/webhooks.md`), ghi lại `X-Webhook-Id`, timestamp và kết quả
- [x] Smoke: tạo webhook, phát sinh sự kiện, chờ nhận, verify đúng; gửi lại chính request đó sau khi quá tolerance thì bị từ chối

**Xong khi:** bước chạy được bằng một lệnh và phương án đã chọn được ghi rõ.

### G4. Chạy lại và ghi kết quả

**File:** `docs/smoke-test.md`, `docs/security.md`.

- [x] Người dùng chạy smoke đầy đủ trên commit candidate của v1.1.1 và dán đầu ra (ngày, commit, kết quả)
- [x] Giữ kết quả cũ (commit `4de7e5d`) dưới mục Lịch sử, không xóa
- [x] Chạy lại govulncheck, gosec, trivy, gitleaks và cập nhật bảng kết quả

**Xong khi:** mọi kết quả gắn với commit thật. Agent chỉ chuẩn bị mẫu và chỗ điền.

## Giai đoạn H: Nhất quán dữ liệu và chính sách quyền

### H1. Transaction cho đổi quyền thành viên

**File:** `internal/service/project_service.go`, test service và integration.

- [x] `RemoveMember` dùng `WithTx`: xóa member, vô hiệu webhook và gỡ assignee trong cùng transaction; chỉ xóa cache sau khi commit
- [x] `UpdateMemberRole` khi hạ xuống viewer cũng dùng transaction
- [x] Test: ép lỗi ở bước thứ hai hoặc thứ ba thì rollback, không bước nào được giữ lại

**Xong khi:** test rollback xanh.

### H2. Hạn mức attachment nguyên tử

**File:** `internal/service/attachment_service.go`, `queries/task_attachments.sql`, test integration.

- [x] Chọn thiết kế và ghi lý do trong commit: (a) đặt chỗ bằng bản ghi trạng thái pending rồi mới upload, hoặc (b) khóa ngắn quanh bước kiểm tra quota và insert bằng `pg_advisory_xact_lock` theo user; không giữ khóa suốt lúc upload
- [x] Lỗi sau khi đặt chỗ phải dọn bản ghi/object (tái dùng job dọn object mồ côi)
- [x] Test integration: N upload song song với quota N-1 thì đúng N-1 thành công

**Xong khi:** test song song xanh với `-race`.

### H3a. Chốt chính sách quản lý webhook (ADR)

**File:** `docs/adr/0019-webhook-management-policy.md`.

- [x] Ai được xem, sửa, xóa, bật/tắt webhook của project và xem deliveries: chỉ người tạo, owner hay cả editor?
- [x] Webhook của member đã rời: tự xóa, hay giữ ở trạng thái vô hiệu và chỉ owner quản lý
- [x] Ghi bảng quyền và lý do từng quyết định

**Xong khi:** chỉ có file ADR, không đụng code.

### H3b. Áp dụng chính sách webhook

**Điều kiện:** sau H3a.

**File:** `internal/service/webhook_service.go`, `internal/transport/http/handler/webhook.go`, test, `docs/webhooks.md`.

- [ ] Sửa `authorizeWebhookAccess` theo ADR-0019; người không có quyền nhận 404
- [ ] Test ma trận: người tạo, owner, editor khác, viewer, member đã rời nhân với get, update, delete, deliveries, test ping

**Xong khi:** test xanh, tài liệu cập nhật.

## Giai đoạn I: Dọn dẹp và chuỗi cung ứng

### I1. Xóa mã cũ không còn dùng

**File:** `pkg/webhook/security.go` và test liên quan.

- [ ] Tìm bằng grep `ComputeSignature` và các hàm/header kiểu cũ; xóa nếu chỉ còn test tham chiếu
- [ ] Chạy golangci-lint bật `unused` để bắt thêm mã chết; chỉ xóa phần chắc chắn không dùng

**Xong khi:** lint và test xanh, hành vi không đổi.

### I2. Ghim toolchain Go và công cụ quét

**File:** `go.mod`, `Dockerfile`, `.github/workflows/ci.yml`, `docs/security.md`.

- [ ] Thêm `toolchain go1.x.y` (bản vá đã sửa các advisory stdlib nêu trong security.md) và build image bằng đúng phiên bản đó
- [ ] CI kiểm tra `go version` khớp `go.mod`; sửa ghi chú trong `security.md` (bỏ câu giảm thiểu chung chung)
- [ ] Ghim phiên bản gosec và gitleaks trong CI và trong lệnh ví dụ của tài liệu

**Xong khi:** govulncheck không còn báo advisory stdlib.

## Giai đoạn K: Phát hành v1.1.1

### K1. Chuẩn bị phát hành

**File:** `CHANGELOG.md`, `README.md`, `docs/`.

- [ ] CHANGELOG v1.1.1: Fixed (tag image mặc định, route MinIO, transaction thành viên, quota nguyên tử), Changed (chính sách webhook), Security
- [ ] README: cập nhật tag mẫu, hướng dẫn chạy smoke test, link ADR-0019
- [ ] Khi hoàn tất, chuyển tài liệu kế hoạch này vào `docs/archive/`
- [ ] Agent chỉ chuẩn bị; người dùng tự tag `v1.1.1` sau khi CI xanh, kiểm tra image `1.1.1` kéo về được và tạo GitHub Release

**Xong khi:** README khớp với code, không có tag cũ nào bị dời.

## Định nghĩa hoàn thành v1.1.1

- [ ] Compose prod không còn tag mặc định cũ
- [ ] Smoke test có bước cách ly rate limit, giao webhook thật kèm verify, chặn method MinIO; kết quả ghi theo commit thật
- [ ] Đổi quyền thành viên là một transaction, quota upload đúng khi chạy song song
- [ ] Chính sách webhook ghi trong ADR-0019 và có bảng test
- [ ] CI xanh trên tag `v1.1.1`, image `1.1.1` có trên GHCR

## Ngoài phạm vi (để sau)

- Mã hóa secret webhook khi lưu DB và trong payload Asynq (AES-GCM, truyền `webhook_id` thay cho secret)
- Outbox pattern cho sự kiện webhook
- RBAC nâng cao, khóa tài khoản theo từng account sau nhiều lần đăng nhập sai
