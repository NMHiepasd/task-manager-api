# ADR 016: Task File Attachments with MinIO / S3 Object Storage

## Status
Accepted

## Context
Collaborative task management systems require file sharing capabilities (documents, screenshots, diagrams, logs, specifications) directly attached to tasks. Storing binary assets directly in PostgreSQL (e.g. `BYTEA` or large objects) bloats database backups, degrades table scan performance, and complicates horizontal scaling.

Key requirements:
1. **Object Storage Decoupling**:
   - Store binary payloads in S3-compatible object storage (MinIO for local development / testing, AWS S3 / Cloudflare R2 / GCS in cloud environments).
   - Only store file metadata (UUID, filename, byte size, MIME content type, object key, timestamps) in PostgreSQL.
2. **Security & File Sanitization**:
   - Strictly prohibit executable or dangerous extensions (`.exe`, `.sh`, `.bat`, `.cmd`, `.msi`, `.vbs`, `.jar`, `.iso`).
   - Sanitize original filenames to eliminate directory traversal risks (`..`, invalid separators).
   - Enforce configurable file size ceilings (default: 10MB per file) and dynamic request body size limits in Gin middleware.
   - Use partitioned, content-addressable storage keys: `tasks/{task_id}/{attachment_uuid}_{sanitized_filename}`.
3. **Access Control & Anti-IDOR**:
   - Inherit task permissions: only authorized users (creator, assignee, or project members with `owner` / `editor` role) can upload files. Viewers have read-only download access.
   - Non-authorized users receive `404 Not Found` to prevent revealing task existence.
4. **Moderation & Deletion**:
   - Allowed deletion for:
     - The uploader (managing their own attachments).
     - The task creator (governing task assets).
     - The project owner (governing all project workspace contents).
   - Deletion removes the database metadata record and purges the object from storage.
5. **Download Options**:
   - Direct streaming (`GET /api/v1/tasks/:id/attachments/:attachmentId`) with proper `Content-Disposition` and `Content-Type` headers.
   - Temporary presigned URLs (`GET /api/v1/tasks/:id/attachments/:attachmentId?presigned=true`) offloading heavy bandwidth to object storage.

## Decision
1. **Storage Provider & SDK**:
   - Integrated `cgr.dev/chainguard/minio:latest` into `docker-compose.yml` (zero-CVE, hardened OCI image).
   - Implemented `StorageClient` abstraction interface and `MinIOClient` using official `github.com/minio/minio-go/v7`.
   - Automatic bucket initialization (`EnsureBucket`) on application startup.
2. **Database Schema**:
   - Migration `000006_create_task_attachments.up.sql` creates table `task_attachments` with foreign keys cascading from `tasks` and `users`.
   - Composite index `(task_id, created_at ASC)` for fast chronological feeds and queries.
3. **HTTP Transport**:
   - `POST /api/v1/tasks/:id/attachments`: Multipart file upload.
   - `GET /api/v1/tasks/:id/attachments`: List task attachments metadata.
   - `GET /api/v1/tasks/:id/attachments/:attachmentId`: Download or presigned URL retrieval.
   - `DELETE /api/v1/tasks/:id/attachments/:attachmentId`: Remove attachment.

## Consequences
### Positive
- High-performance binary storage offloaded from PostgreSQL to scalable S3 object storage.
- Storage interface abstraction allows seamless swap between MinIO, AWS S3, Google Cloud Storage, or Ceph.
- Automatic orphan rollback: if DB insert fails after storage upload, the uploaded object is immediately purged from storage.
- Anti-IDOR and moderation protections maintain zero-trust security boundaries.

### Negative / Trade-offs
- Requires MinIO or S3 endpoint dependency when running full docker compose stack (gracefully skipped if storage is unconfigured).
