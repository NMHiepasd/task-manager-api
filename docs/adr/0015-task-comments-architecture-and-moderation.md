# ADR 015: Task Comments Architecture and Moderation

## Status
Accepted

## Context
Collaborative project and task tracking requires asynchronous discussions directly attached to individual tasks. Team members need to ask questions, share status updates, and document decisions within the context of a task.

Key requirements:
1. **Access Control & Least Privilege**:
   - Comments must inherit task visibility: only users with access to a task (task creator, assignee, or members of the task's project) can view and add comments.
   - Non-authorized users receive `404 Not Found` (anti-IDOR) rather than `403 Forbidden` to prevent leaking task existence.
2. **Authorship & Moderation**:
   - Only the author of a comment can edit its content (`PATCH /api/v1/tasks/:id/comments/:commentId`).
   - Deletion (`DELETE /api/v1/tasks/:id/comments/:commentId`) is allowed for:
     - The comment author (managing their own messages).
     - The task creator (moderating discussion on their task).
     - The project owner (moderating content across their project workspace).
3. **Scalability & Performance**:
   - Tasks may accumulate hundreds of comments over time. The system requires both keyset cursor-based pagination and traditional page-based pagination.
   - Composite index `(task_id, created_at ASC)` guarantees sub-millisecond retrieval ordered chronologically.

## Decision
1. **Schema & Migration**:
   - Created table `task_comments` in `000005_create_task_comments.up.sql` with UUID v7 primary keys and timestamps.
   - Foreign keys to `tasks(id)` and `users(id)` with `ON DELETE CASCADE`.
   - Indexes on `(task_id, created_at ASC)` and `(user_id)`.
2. **Clean Architecture Service Layer**:
   - Created dedicated `CommentService` in `internal/service/comment_service.go` separating comment domain logic from task scheduling.
   - Enforced validation rules (non-empty trimmed content, max 2,000 characters) via DTO binding and RFC 9457 error handling.
3. **REST Endpoints**:
   - `POST /api/v1/tasks/:id/comments`: Create comment.
   - `GET /api/v1/tasks/:id/comments`: List comments with pagination (keyset cursor or offset).
   - `PATCH /api/v1/tasks/:id/comments/:commentId`: Update comment (author only).
   - `DELETE /api/v1/tasks/:id/comments/:commentId`: Delete comment (author, task creator, or project owner).

## Consequences
### Positive
- Fully integrates with the RBAC model established in ADR-014.
- Facilitates team discussion without external messaging overhead.
- Moderation privileges enable maintainers to clean up stale or inappropriate content.
- 100% covered by property fuzzing and contract tests in Schemathesis.

### Negative / Trade-offs
- Comment listing performs an indexed join on `users` to provide author details (`full_name`, `email`) in a single query.
