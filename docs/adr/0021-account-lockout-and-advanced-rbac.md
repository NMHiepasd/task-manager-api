# ADR 0021: Account Lockout Policy and Advanced Role-Based Access Control (RBAC)

## Status
Accepted

## Context

### 1. The Distributed Brute-Force Vulnerability
In Task Manager API v1.1.0, brute-force and credential stuffing threats are primarily mitigated by IP-based rate limiting (`pkg/limiter`, ADR 0003), which restricts requests per IP address per time window (e.g., 5 login requests per minute per IP).

However, modern credential stuffing and targeted brute-force attacks frequently utilize distributed botnets with residential proxy pools rotating across thousands of discrete IP addresses. In such scenarios:
- Every individual request originates from a distinct IP address, easily bypassing IP-level rate limits.
- An attacker targeting a specific user's email can execute unbounded login attempts over time without triggering IP-based thresholds.
- Without account-level protections, user accounts remain susceptible to password guessing and dictionary attacks.

### 2. Enterprise Auditing & Compliance Requirements
The existing user authorization scheme only distinguishes between two system roles: `user` and `admin`:
- `admin` possesses unrestricted access across resources.
- `user` manages personal tasks, projects, comments, attachments, and webhooks.

In enterprise and regulated environments (SOC 2, ISO 27001, GDPR), security policies mandate a strict **Separation of Duties (SoD)** and the **Principle of Least Privilege (PoLP)**:
- Security officers and external compliance auditors need visibility into system health, audit logs, Prometheus metrics, and webhook delivery lifecycles to assess security posture and trace anomalies.
- However, auditors must **never** possess write permissions, must not be able to mutate configurations or business data, and must not have unauthorized read access to personal task descriptions or private workspace content.

---

## Decision

We establish two complementary security mechanisms:
1. **Account Lockout Policy**: A stateful, time-bound defense mechanism against distributed brute-force attacks, incorporating timing-attack resistance and self-healing auto-recovery.
2. **Advanced Role-Based Access Control (RBAC)**: An enhanced permission model introducing the `auditor` system role, clear separation between system-level and project-level scopes, and an explicit permissions matrix.

---

### Part 1: Account Lockout Specification

#### 1. Threshold and Lockout Duration
- **Threshold**: 5 consecutive failed login attempts (matching on user email).
- **Lockout Duration**: 15 minutes (`locked_until = NOW() + INTERVAL '15 minutes'`).
- **Reset Trigger**: Upon successful authentication with valid credentials, the failure counter is reset to `0` and `locked_until` is cleared (`NULL`).

#### 2. Self-Healing Auto-Recovery
- When the 15-minute lockout period expires (`locked_until <= NOW()`), the user account is automatically eligible for login attempts without requiring administrator intervention.
- If the subsequent login attempt succeeds:
  - `failed_login_attempts` is reset to `0`.
  - `locked_until` is set to `NULL`.
- If the subsequent attempt fails:
  - The failure counter resets to `1` (or increments) and sets `last_failed_login_at = NOW()`.

#### 3. Database Schema Modifications (`users`)
Migration `migrations/000011_add_account_lockout_to_users.up.sql` will add the following columns to the `users` table:

```sql
ALTER TABLE users
    ADD COLUMN failed_login_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN locked_until TIMESTAMPTZ NULL,
    ADD COLUMN last_failed_login_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_users_locked_until 
    ON users(locked_until) 
    WHERE locked_until IS NOT NULL;
```

#### 4. Concurrency & Atomic State Transitions
To prevent race conditions during rapid concurrent login attempts, counter increments and lockout assessments are executed atomically within PostgreSQL:

```sql
-- queries/users.sql
-- name: RecordFailedLogin :one
UPDATE users
SET failed_login_attempts = failed_login_attempts + 1,
    last_failed_login_at = CURRENT_TIMESTAMP,
    locked_until = CASE
        WHEN failed_login_attempts + 1 >= 5 THEN CURRENT_TIMESTAMP + INTERVAL '15 minutes'
        ELSE locked_until
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING failed_login_attempts, locked_until;

-- name: ResetFailedLogin :exec
UPDATE users
SET failed_login_attempts = 0,
    locked_until = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;
```

#### 5. Defense Against Timing Attacks & User Enumeration
When an attacker attempts to log in:
1. **Non-Existent User**:
   - Execute dummy password hashing with `s.dummyHash` (same bcrypt cost factor) to simulate execution duration.
   - Return generic error: `401 Unauthorized` (`ErrInvalidCredentials`).
2. **Active Locked Account (`locked_until > NOW()`)**:
   - To prevent attackers from differentiating between valid credentials and invalid credentials on a locked account via response timing, the system **always executes dummy password comparison** before returning the lockout response.
   - Return standard RFC 9457 error: `423 Locked` (`ErrAccountLocked`).
3. **Invalid Password on Unlocked Account**:
   - Compare password hash with bcrypt.
   - Call `RecordFailedLogin(ctx, user.ID)`.
   - Return `401 Unauthorized` (`ErrInvalidCredentials`).

#### 6. API Error Contract (RFC 9457 Problem Details)
When an account is locked, the API responds with HTTP status code `423 Locked`:

```json
{
  "type": "https://api.taskmanager.com/errors/account-locked",
  "title": "Account Locked",
  "status": 423,
  "code": "ACCOUNT_LOCKED",
  "detail": "Account is temporarily locked due to 5 consecutive failed login attempts. Please try again after 15 minutes.",
  "instance": "/api/v1/auth/login",
  "invalid_params": null
}
```

---

### Part 2: Advanced RBAC Specification

#### 1. System Roles Hierarchy & Definitions
The system distinguishes three global roles defined in `users.role`:

1. **`admin`**: System administrator.
   - Full control over system configurations, user provisioning, role assignments, audit logs, and operational metrics.
2. **`auditor`**: Compliance and security inspector.
   - Read-only access to audit logs, Prometheus metrics (`/metrics`), system health, and webhook delivery logs.
   - **Zero write privileges**: Prohibited from modifying any data, updating user roles, or triggering business mutations.
   - Prohibited from reading private user tasks, personal attachments, or project board contents (strictly separation of duty).
3. **`user`**: Standard authenticated member.
   - Can manage own tasks, projects, comments, personal webhooks, and attachments.
   - Access to collaborative projects governed by Project RBAC (`owner`, `editor`, `viewer`).

#### 2. System-Level vs. Project-Level Authorization
Task Manager API enforces a two-tier authorization model:

```
+-------------------------------------------------------------------+
|                        System-Level (RBAC)                        |
|                     (users.role in JWT Claims)                     |
|            +---------------+---------------+---------------+      |
|            |     admin     |    auditor    |     user      |      |
|            +---------------+---------------+---------------+      |
+------------------------------------+------------------------------+
                                     | (For Project Resources)
                                     v
+-------------------------------------------------------------------+
|                       Project-Level (Workspace)                    |
|                    (project_members.role in DB)                   |
|            +---------------+---------------+---------------+      |
|            |     owner     |    editor     |    viewer     |      |
|            +---------------+---------------+---------------+      |
+-------------------------------------------------------------------+
```

- **System-Level Roles** are embedded in JWT Access Token claims (`role: "admin" | "auditor" | "user"`). Middlewares (`RequireRole`, `RequireCapability`) validate global access without redundant DB queries.
- **Project-Level Roles** (`owner`, `editor`, `viewer`) control task operations inside workspaces via `authz.AuthorizeTaskAccess` (ADR 0018).

#### 3. Capabilities & Scopes Matrix

| Capability / Resource Scope | Action | `admin` | `auditor` | `user` |
| :--- | :--- | :---: | :---: | :---: |
| **System Metrics** (`/metrics`) | Read | :white_check_mark: | :white_check_mark: | :x: |
| **Health & Readiness** (`/healthz`, `/readyz`) | Read | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| **User Directory** (`/api/v1/users`) | Read | :white_check_mark: | :white_check_mark: | :x: |
| **User Management** (Role update, lock/unlock) | Write | :white_check_mark: | :x: | :x: |
| **Audit Logs / Activity Trail** | Read | :white_check_mark: | :white_check_mark: | :x: |
| **Webhook Deliveries Inspection** | Read | :white_check_mark: | :white_check_mark: | Own Only |
| **Webhook Endpoints Management** | Write | :white_check_mark: | :x: | Own Only |
| **Projects & Tasks Execution** | Create / Edit / Delete | Own / Manage | :x: | Member Role |
| **Task Viewing (Personal / Workspace)** | Read | Own / Member | :x: | Member Role |
| **Comments & Attachments** | Read / Write | Own / Member | :x: | Member Role |

#### 4. Schema Constraint Update
Migration `000011` will update the `users` table constraint:
```sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin', 'auditor'));
```

---

## Consequences

### Positive
- **Targeted Brute-Force Immunity**: Distributed botnets attempting password guessing against individual accounts are arrested after 5 attempts for 15 minutes.
- **Zero Operator Burden**: Self-healing auto-recovery removes the need for customer support or administrative unlocks for temporary lockouts.
- **Timing Attack Safety**: Dummy password comparisons ensure response latency remains uniform across existent, nonexistent, and locked accounts.
- **Regulatory Compliance**: The addition of the `auditor` role enables independent auditability and SOC 2 / ISO 27001 readiness without granting data modification powers.
- **Clear Error Semantics**: Using HTTP 423 `Locked` adheres strictly to RFC standards and RFC 9457 error contracts.

### Negative & Mitigations
- **Denial of Service (Account DoS) Risk**: A malicious actor knowing a target's email address could intentionally fail logins to lock the legitimate user out.
  - *Mitigation*: The lockout duration is limited to 15 minutes (rather than indefinite).
  - *Mitigation*: IP-based rate limiting (`pkg/limiter`) restricts rapid sequential requests per IP, preventing trivial automated harassment.
  - *Mitigation*: Successful password authentication resets the window, and future versions can support email magic link bypass.

---

## Implementation Plan

1. **Task A1 (Current)**: Architecture Decision Record ADR-0021 (this document).
2. **Task A2**: Database migration `000011_add_account_lockout_to_users.up.sql`, `queries/users.sql` updates, and sqlc generation.
3. **Task A3**: Domain error definition `ErrAccountLocked` (HTTP 423), integration into `AuthService.Login`, timing attack defense, and HTTP handler integration.
4. **Task A4**: Integration test suite verifying:
   - 4 consecutive failed logins $\to$ success resets counter.
   - 5th failed login triggers account lockout.
   - Active locked account returns HTTP 423.
   - Dummy comparison execution prevents timing discrepancy.
   - Successful login after 15-minute expiration.
   - Auditor role permissions enforcement on endpoints.
