# ADR 019: Webhook Management Policy, Scoping, and Authorization Matrix

## Status
Accepted

## Context
In ADR-017, the asynchronous webhook notification system was introduced with Asynq, Redis, and HMAC-SHA256 signatures. In ADR-018, the project security boundary and task access policies were formalized.

However, subsequent security reviews of the v1.1.0 release identified critical authorization and lifecycle ambiguities in webhook management:
1. **Broken Access Control on Former Members**: `authorizeWebhookAccess` previously granted unconditional access if `wh.UserID == userID`. As a result, a collaborator who had been removed from a project (or voluntarily left) could still query, update, re-activate, trigger test pings, or delete webhooks associated with that project.
2. **Peer Editor Tampering**: In project-scoped webhooks, any collaborator with the `editor` role could modify or delete webhooks configured by other editors or by the project owner. Webhook target URLs frequently contain pre-shared authorization tokens (e.g., Slack incoming webhook URLs, Discord webhook tokens, internal microservice tokens). Permitting peer editors to inspect or alter each other's webhooks exposes these credentials and allows integration hijacking.
3. **Lifecycle Handling on Role Demotion or Member Removal**: When a member is demoted to `viewer` or removed from a project, outgoing webhooks created by them could continue exfiltrating sensitive project events (such as task descriptions, comments, and attachment notifications) to external servers outside the project's trust boundary.
4. **Ambiguity on Deletion vs. Deactivation**: When an integration author leaves the organization, deleting their webhooks outright causes immediate pipeline breakage and permanently destroys delivery audit logs (`webhook_deliveries`).

This record defines the authoritative Webhook Management Policy and Permission Matrix to resolve these issues prior to implementation in Task H3b.

---

## Decision

### 1. Dual Scoping Model: Personal Webhooks vs. Project Webhooks

All webhooks belong to one of two mutually exclusive scopes:

1. **Personal Webhooks (`project_id IS NULL`)**:
   - Scoped strictly to the individual user account (`wh.user_id`).
   - Only receives notifications for personal tasks owned or assigned to the user.
   - The creator is the sole administrator. Any other user receives **HTTP 404 (Not Found)**.

2. **Project Webhooks (`project_id IS NOT NULL`)**:
   - Scoped strictly to the project security boundary (`wh.project_id`).
   - Receives events for all tasks, comments, attachments, and membership changes within that project.
   - Access control is governed by the project's Role-Based Access Control (RBAC) in conjunction with creator identity.

---

### 2. Project Webhook Roles and Permissions

For any project-scoped webhook, the caller falls into one of the following roles:
- **Project Owner**: The administrative owner of the parent project (`project.owner_id == caller_id`).
- **Creator (Active Editor / Owner)**: The user who created the webhook (`wh.user_id == caller_id`) AND currently holds an active `editor` or `owner` role in the project.
- **Peer Editor (Non-Creator)**: An active project member with `editor` role who did NOT create the webhook (`wh.user_id != caller_id`).
- **Project Viewer**: An active project member with `viewer` role.
- **Former Member / Outsider**: A user who has left the project, been removed, or has never been a member of the project.

#### Permission Matrix

| Operation | Endpoint | Project Owner | Creator (Active Editor/Owner) | Peer Editor | Project Viewer | Former Member / Outsider | Personal Webhook: Creator | Personal Webhook: Outsider |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`CreateWebhook`** | `POST /api/v1/webhooks` | **ALLOW** | **ALLOW** | **ALLOW** | DENY (403) | DENY (404) | **ALLOW** | DENY (404) |
| **`GetWebhook`** | `GET /api/v1/webhooks/:id` | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |
| **`UpdateWebhook`** | `PATCH /api/v1/webhooks/:id` | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |
| **`ToggleActive`** | `PATCH /api/v1/webhooks/:id` (`is_active`) | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |
| **`DeleteWebhook`** | `DELETE /api/v1/webhooks/:id` | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |
| **`TriggerTestPing`**| `POST /api/v1/webhooks/:id/test` | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |
| **`ListDeliveries`** | `GET /api/v1/webhooks/:id/deliveries` | **ALLOW** | **ALLOW** | DENY (404) | DENY (404) | DENY (404) | **ALLOW** | DENY (404) |

#### Rationale for Access Rules
1. **Project Owner Privileges**: Project owners are accountable for all outbound data leaving the project. They require complete visibility, moderation, toggle, and cleanup authority over all integrations.
2. **Peer Editor Restriction (Anti-IDOR / Anti-Tampering)**: Restricting peer editors from reading or modifying webhooks created by other editors protects embedded secret tokens in webhook URLs (e.g., Slack Webhook URLs with secret path components) and eliminates sabotage or accidental misconfiguration.
3. **Consistent Anti-IDOR Response (HTTP 404)**: When a caller attempts to access a webhook they are not authorized to manage or view, the API consistently returns `404 Not Found` (rather than `403 Forbidden`). This prevents malicious actors from discovering the existence of internal webhook IDs or probing for private endpoints.

---

### 3. Lifecycle Policy for Departed or Demoted Members

When a project member's role changes or their membership terminates, the following lifecycle rules apply:

1. **Immediate Atomic Deactivation**:
   - As implemented in Task H1 (`RemoveMember` and `UpdateMemberRole`), any demotion to `viewer` or removal from `project_members` executes `DeactivateProjectWebhooksByUser` inside the same database transaction.
   - All webhooks created by that user for that project are immediately marked `is_active = false`.
   - **No Automated Hard Deletion**: The webhook record and its historical deliveries (`webhook_deliveries`) are preserved intact.

2. **Management Custody Transferred to Project Owner**:
   - The former member loses all access immediately (`wh.UserID == userID` check is superseded by the project membership check; returns 404).
   - Only the **Project Owner** retains access to the deactivated webhook.
   - The Project Owner can:
     - Review the webhook configuration and delivery history.
     - Update the target URL, regenerate secret, and re-activate the webhook (`is_active = true`) if the integration serves a vital organizational function (e.g., team chat or CI channel).
     - Or permanently delete the webhook (`DELETE /api/v1/webhooks/:id`).

3. **Why Not Auto-Delete?**:
   - **Audit Trail Integrity**: Deleting webhooks cascades and purges `webhook_deliveries`, erasing forensic history of which events were dispatched.
   - **Business Continuity**: Crucial shared integrations (e.g., Slack alerts, PagerDuty, deployment hooks) should not suddenly fail across the entire team simply because the engineer who initially registered the hook transitioned to another team or departed.
   - **Zero Risk of Exfiltration**: Because the webhook is deactivated atomically, no events can be dispatched until the Project Owner explicitly reviews and re-enables it.

---

## Consequences

### Positive
- **Guaranteed Zero Data Exfiltration**: Departed or demoted members cannot receive project events or re-activate dormant webhooks.
- **Protection of Sensitive Integration Secrets**: Peer editors cannot view target URLs containing embedded tokens or modify fellow team members' integrations.
- **Audit Preservation**: Complete delivery audit logs remain available to project owners even after team turnover.
- **Operational Continuity**: Project owners have full autonomy to inspect, adopt, reconfigure, or purge legacy integrations.
- **Strict Anti-IDOR**: Unauthorized requests consistently receive 404 Not Found, leaking zero metadata about existing subscriptions.

### Negative / Trade-offs
- Peer editors cannot collaborate on managing a shared webhook without transferring project ownership or asking the original creator.
- If a departed member had critical webhooks, the Project Owner must manually inspect and re-activate them.
