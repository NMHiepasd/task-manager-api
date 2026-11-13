# ADR 014: Project Sharing and Role-Based Access Control (RBAC)

## Status
Accepted

## Context
In previous phases, projects were strictly single-user owned: only the project owner could view or manage tasks within a project. To enable multi-user teamwork and project delegation, the API requires collaboration features where users can share projects with colleagues and assign granular roles.

Key requirements:
1. Multiple collaborators per project with explicit roles (`owner`, `editor`, `viewer`).
2. Least privilege principle:
   - `owner`: full control over project metadata, member management (invite, change role, remove), and tasks. Cannot be removed or demoted.
   - `editor`: can create, view, update, delete tasks within the project; cannot manage project settings or members.
   - `viewer`: read-only access to project and tasks; cannot mutate tasks or members.
3. Clean user experience: invite members by email rather than requiring users to manually exchange UUIDs.
4. Seamless integration with task authorization: operations on project tasks (`CreateTask`, `GetTaskByID`, `UpdateTask`, `DeleteTask`, `SetTaskTags`) must verify the caller's role within the task's project.
5. Anti-IDOR: Unauthorized access attempts return 404 Not Found rather than 403 Forbidden to prevent resource enumeration.

## Decision
1. **Schema & Migration**:
   - Added table `project_members` with composite unique constraint `(project_id, user_id)`.
   - Indexed on `(project_id)` and `(user_id)`.
   - Included automatic backfill migration in `000004_create_project_members.up.sql` to register all existing project owners into `project_members` with role `owner`.
   - Auto-registration of `owner` role during `CreateProject`.
2. **Project Visibility**:
   - Updated `ListProjects` to join `projects` with `project_members` for the caller, exposing projects where caller is owner or collaborator.
   - Keyset and offset pagination both support `member_role` emission.
3. **Collaboration Endpoints**:
   - `POST /api/v1/projects/:id/members`: Add collaborator by email (owner only, role `editor` | `viewer`).
   - `GET /api/v1/projects/:id/members`: List project collaborators (accessible to all project members).
   - `PATCH /api/v1/projects/:id/members/:userId`: Update member role (owner only).
   - `DELETE /api/v1/projects/:id/members/:userId`: Remove collaborator (owner can remove members; members can leave project).
4. **Task Access Control**:
   - `CreateTask`: Caller must have `owner` or `editor` role in target project.
   - `GetTaskByID`: Caller must be creator, assignee, or member (`owner`, `editor`, `viewer`) of the project.
   - `UpdateTask` / `DeleteTask` / `SetTaskTags`: Caller must be creator, assignee, or member with `owner` / `editor` role.

## Consequences
### Positive
- Fully supports team collaboration and project sharing.
- Granular permissions prevent unauthorized modification of tasks or project metadata.
- Email-based invites make the API practical for frontend and third-party integrations.
- Foundation established for downstream collaboration features (Task Comments, File Attachments, Activity Logs).

### Negative / Trade-offs
- Task queries check project membership via indexed lookup or join.
- Project membership changes require cache invalidation for the target user's project cache.
