package domain

// System role definitions.
const (
	RoleAdmin   = "admin"
	RoleAuditor = "auditor"
	RoleUser    = "user"
)

// System permission capabilities.
const (
	PermissionUsersRead      = "users:read"
	PermissionUsersManage    = "users:manage"
	PermissionAuditRead      = "audit:read"
	PermissionSystemMetrics  = "system:metrics"
	PermissionWebhooksRead   = "webhooks:read"
	PermissionWebhooksManage = "webhooks:manage"
	PermissionTasksRead      = "tasks:read"
	PermissionTasksWrite     = "tasks:write"
	PermissionProjectsRead   = "projects:read"
	PermissionProjectsWrite  = "projects:write"
)

// RolePermissions defines the capabilities matrix mapped from system roles.
var RolePermissions = map[string]map[string]bool{
	RoleAdmin: {
		PermissionUsersRead:      true,
		PermissionUsersManage:    true,
		PermissionAuditRead:      true,
		PermissionSystemMetrics:  true,
		PermissionWebhooksRead:   true,
		PermissionWebhooksManage: true,
		PermissionTasksRead:      true,
		PermissionTasksWrite:     true,
		PermissionProjectsRead:   true,
		PermissionProjectsWrite:  true,
	},
	RoleAuditor: {
		PermissionUsersRead:     true,
		PermissionAuditRead:     true,
		PermissionSystemMetrics: true,
		PermissionWebhooksRead:  true,
	},
	RoleUser: {
		PermissionWebhooksRead:   true,
		PermissionWebhooksManage: true,
		PermissionTasksRead:      true,
		PermissionTasksWrite:     true,
		PermissionProjectsRead:   true,
		PermissionProjectsWrite:  true,
	},
}

// HasPermission checks whether a given system role is granted the specified permission.
func HasPermission(role, permission string) bool {
	if perms, exists := RolePermissions[role]; exists {
		return perms[permission]
	}
	return false
}

// IsValidRole validates whether a role string matches a recognized system role.
func IsValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleAuditor, RoleUser:
		return true
	default:
		return false
	}
}
