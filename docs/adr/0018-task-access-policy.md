# ADR 018: Task Access Policy and Permission Matrix

## Status
Accepted

## Context
In previous phases, task access control, task comments, and file attachments were introduced across separate architectural milestones:
- **ADR-004**: Baseline single-user task access and Anti-IDOR policy.
- **ADR-014**: Project sharing and Role-Based Access Control (RBAC) with `owner`, `editor`, and `viewer` roles.
- **ADR-015**: Task comments architecture, chronological feed, and moderation rights.
- **ADR-016**: Task file attachments with S3/MinIO object storage, size constraints, and deletion rights.

As features expanded across `TaskService`, `CommentService`, and `AttachmentService`, authorization checks were implemented independently within each service layer. This introduced subtle inconsistencies and edge-case ambiguities:
1. **Creator vs. Project Role**: Does an original task creator retain modification or moderation rights if their role in the project is later demoted to `viewer`, or if they are removed from the project entirely?
2. **Viewer Permissions**: Can project viewers contribute comments to discuss progress or ask questions, or are they strictly read-only across all sub-resources? Can they upload attachments?
3. **Personal Task Assignees**: What exact subset of permissions does an assignee have on a standalone personal task (`project_id == nil`)?
4. **Member Removal Side Effects**: When a collaborator is removed from a project (or voluntarily leaves), what happens to tasks currently assigned to them (`assignee_id`)?
5. **Moderation Boundary**: Can a `project_editor` delete comments or attachments created by other team members, or is cross-user moderation reserved exclusively for the project owner?

To prevent authorization drift, eliminate privilege escalation bugs, and establish a single source of truth prior to centralizing authorization logic in `internal/authz`, this record formalizes the complete Task Access Policy and Permission Matrix.

---

## Decision

### 1. Dual Domain Model: Project Tasks vs. Personal Tasks

All tasks belong to one of two mutually exclusive domains:

1. **Project Tasks (`project_id IS NOT NULL`)**:
   - The project is the **sole security boundary**.
   - Access is derived exclusively from the caller's active role in `project_members`: `owner`, `editor`, or `viewer`.
   - **Creator identity does NOT grant superuser privileges or bypass project RBAC**. If the user who created the task is later demoted to `viewer`, they have viewer-level permissions. If they are removed from the project (`outsider`), they lose all access immediately.
   - If the parent project is soft-deleted (`deleted_at IS NOT NULL`), the task is treated as nonexistent.

2. **Personal Tasks (`project_id IS NULL`)**:
   - The task belongs entirely to the `creator_id`.
   - The `creator` has full administrative ownership over the task and its sub-resources.
   - The task may optionally be assigned to another user (`assignee_id`).
   - The `assignee` is granted collaborative operational rights (read, comment, upload) but cannot alter ownership, delete the task, or moderate the creator's content.
   - Any other user is an `outsider`.

---

### 2. Comprehensive Permission Matrix

The following matrix defines permitted actions for each role across Project Tasks and Personal Tasks:

| Action | Description | Project: `owner` | Project: `editor` | Project: `viewer` | Project: `outsider` | Personal: `creator` | Personal: `assignee` | Personal: `outsider` |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`Read`** | View task details, list tags, list comments, view/download attachments | **ALLOW** | **ALLOW** | **ALLOW** | DENY (404) | **ALLOW** | **ALLOW** | DENY (404) |
| **`Comment`** | Post a new comment on the task | **ALLOW** | **ALLOW** | **ALLOW** | DENY (404) | **ALLOW** | **ALLOW** | DENY (404) |
| **`Edit`** | Update task fields (title, description, status, priority, due date, tags, assignee) | **ALLOW** | **ALLOW** | DENY (403) | DENY (404) | **ALLOW** | DENY (403) | DENY (404) |
| **`Delete`** | Soft delete or purge the task | **ALLOW** | **ALLOW** | DENY (403) | DENY (404) | **ALLOW** | DENY (403) | DENY (404) |
| **`Upload`** | Upload new file attachment to the task | **ALLOW** | **ALLOW** | DENY (403) | DENY (404) | **ALLOW** | **ALLOW** | DENY (404) |
| **`ModerateComments`** | Delete comments authored by *other* users | **ALLOW** | DENY (403) | DENY (403) | DENY (404) | **ALLOW** | DENY (403) | DENY (404) |
| **`ModerateAttachments`** | Delete attachments uploaded by *other* users | **ALLOW** | DENY (403) | DENY (403) | DENY (404) | **ALLOW** | DENY (403) | DENY (404) |

#### Rules for Self-Authored Sub-Resources
- **Own Comments**: Any user with active `Read` permission on the task may edit (`PATCH`) or delete (`DELETE`) **their own** comments.
- **Own Attachments**: Any user with active `Read` permission on the task may delete (`DELETE`) **their own** uploaded attachments.
- If a user loses `Read` access (e.g. removed from the project), they cannot interact with their previously authored comments or attachments (attempts return `404 Not Found`).

---

### 3. Key Policy Determinations

#### Decision 1: Project Viewers and Commenting Rights
- **Outcome**: `project_viewer` **IS PERMITTED** to post comments (`Comment: ALLOW`).
- **Rationale**: In collaborative project management, viewers frequently need to ask clarifying questions, provide status feedback, or review deliverables without having write access to the task parameters.
- **Guardrails**: Viewers **cannot** edit task attributes, delete tasks, upload binary attachments (preserving S3 storage quotas and preventing file spam), or moderate other members' comments.

#### Decision 2: Automated Task Unassignment on Member Removal / Departure
- **Outcome**: When a collaborator is removed from a project or voluntarily leaves via `DELETE /api/v1/projects/:id/members/:userId`:
  ```sql
  UPDATE tasks
  SET assignee_id = NULL
  WHERE project_id = $1 AND assignee_id = $2;
  ```
- **Rationale**: An assigned task represents an active obligation. Retaining an assignment to a user who no longer has access to the project creates orphaned tasks, confuses project reporting, and violates the principle of least privilege.
- **Atomicity**: The unassignment query runs within the same database transaction as the removal from `project_members` to guarantee consistency.

#### Decision 3: Strict Anti-IDOR Strategy (404 Not Found vs. 403 Forbidden)
- **Outcome**: When a caller is an `outsider` (has no access to the task or project), or when the parent project is soft-deleted:
  - The API **MUST** return `404 Not Found` (via RFC 9457 Problem Details) rather than `403 Forbidden`.
  - The response body and error message must not disclose whether the task ID exists.
- **When is 403 Forbidden used?**:
  - `403 Forbidden` is returned **only** when the caller has authenticated and confirmed `Read` membership in the resource domain, but attempts an action prohibited for their specific role (e.g. a `project_viewer` attempting `PATCH /api/v1/tasks/:id` or an `editor` attempting `ModerateComments`).
  - This informs legitimate collaborators of their insufficient permission level without exposing private resources to unauthorized outsiders.

#### Decision 4: Cross-User Content Moderation
- **Outcome**: Only the **`project_owner`** (for project tasks) and the **`creator`** (for personal tasks) have `ModerateComments` and `ModerateAttachments` permissions.
- **Rationale**: Project editors have equal peer status with other editors. Allowing editors to delete comments or attachments created by fellow editors or the owner opens vectors for accidental data loss or griefing. Destructive administrative moderation is strictly reserved for the workspace owner.

---

## Consequences

### Positive
- **Deterministic Security**: Establishes an unambiguous, codified security contract for all developers and automated tests.
- **Zero Ambiguity for `internal/authz`**: Provides the exact truth table for centralized authorization evaluation functions in Task C2.
- **Orphan Prevention**: Automatically cleans up assignee references when users leave projects, maintaining clean project dashboards.
- **Anti-IDOR Hardening**: Protects organizational privacy by preventing enumeration of UUIDs, project scopes, or internal assignments.

### Negative / Trade-offs
- **Additional DB Operation on Member Removal**: Removing a project member incurs an extra `UPDATE tasks` statement within the transaction. However, this is bounded by indexed `(project_id, assignee_id)` and occurs on an infrequent administrative path.
- **Strict Project Separation**: Users who created a task in a project cannot edit it if an owner demotes them to viewer. This is intentional to respect project workspace governance.
