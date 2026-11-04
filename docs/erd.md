# Entity-Relationship Diagram (ERD)

This document describes the relational database schema design for the **Task Manager REST API**.

---

## 1. Mermaid ERD

```mermaid
erDiagram
    USERS ||--o{ REFRESH_TOKENS : "has"
    USERS ||--o{ PROJECTS : "owns"
    USERS ||--o{ TASKS : "creates"
    USERS ||--o{ TASKS : "assigned to"
    USERS ||--o{ TAGS : "owns"
    PROJECTS ||--o{ TASKS : "contains"
    TASKS ||--o{ TASK_TAGS : "labeled with"
    TAGS ||--o{ TASK_TAGS : "applied to"

    USERS {
        uuid id PK "UUID v7"
        varchar email UK "Unique lower(email)"
        varchar password_hash "Argon2id / bcrypt"
        varchar full_name "User name"
        varchar role "user | admin"
        int version "Optimistic lock"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
        timestamptz deleted_at "Soft delete"
    }

    REFRESH_TOKENS {
        uuid id PK "UUID v7"
        uuid user_id FK "References USERS(id)"
        varchar token_hash UK "Hashed refresh token"
        timestamptz expires_at "Expiration timestamp"
        timestamptz revoked_at "Revocation timestamp"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
    }

    PROJECTS {
        uuid id PK "UUID v7"
        uuid owner_id FK "References USERS(id)"
        varchar name "Project name"
        text description "Project description"
        varchar color "Hex color code"
        int version "Optimistic lock"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
        timestamptz deleted_at "Soft delete"
    }

    TASKS {
        uuid id PK "UUID v7"
        uuid project_id FK "Nullable references PROJECTS(id)"
        uuid creator_id FK "References USERS(id)"
        uuid assignee_id FK "Nullable references USERS(id)"
        varchar title "Task title"
        text description "Task details"
        varchar status "todo | in_progress | completed | cancelled"
        varchar priority "low | medium | high | urgent"
        timestamptz due_date "Optional deadline"
        int version "Optimistic lock"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
        timestamptz deleted_at "Soft delete"
    }

    TAGS {
        uuid id PK "UUID v7"
        uuid user_id FK "References USERS(id)"
        varchar name "Tag name"
        varchar color "Hex color code"
        timestamptz created_at "Creation timestamp"
        timestamptz updated_at "Update timestamp"
    }

    TASK_TAGS {
        uuid task_id PK,FK "References TASKS(id)"
        uuid tag_id PK,FK "References TAGS(id)"
    }
```

---

## 2. Key Design Standards

1. **Primary Keys**:
   - Time-ordered UUIDs (UUID v7) generate monotonically increasing IDs that preserve B-Tree index locality and avoid fragmentation compared to random UUID v4.
2. **Timestamps & Timezones**:
   - All datetime fields use `TIMESTAMPTZ` (UTC).
3. **Soft Delete (`deleted_at`)**:
   - Entities (`users`, `projects`, `tasks`) have a nullable `deleted_at` timestamp. Records where `deleted_at IS NULL` are active.
4. **Optimistic Locking (`version`)**:
   - Integer column incremented on every update (`UPDATE ... SET version = version + 1 WHERE id = $1 AND version = $2`). Prevents concurrent overwrite conflicts.
5. **Integrity & Constraints**:
   - Case-insensitive unique emails using a unique index on `lower(email)`.
   - `CHECK` constraints on enumeration fields:
     - `users.role`: `'user'`, `'admin'`
     - `tasks.status`: `'todo'`, `'in_progress'`, `'completed'`, `'cancelled'`
     - `tasks.priority`: `'low'`, `'medium'`, `'high'`, `'urgent'`
   - Foreign key cascading:
     - Deleting a user cascades to their projects, tasks, refresh tokens, and tags.
     - Deleting a project sets `tasks.project_id` to `NULL`.
     - Deleting a task or tag cascades deletion to `task_tags`.
