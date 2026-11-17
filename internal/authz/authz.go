package authz

import (
	"errors"

	"github.com/google/uuid"
)

// Action defines the operation a subject wants to perform on a resource.
type Action string

const (
	// ActionRead permits viewing task details, tags, comments, and attachments.
	ActionRead Action = "Read"
	// ActionComment permits adding a new comment to a task.
	ActionComment Action = "Comment"
	// ActionEdit permits updating task fields (title, description, status, priority, due date, tags, assignee).
	ActionEdit Action = "Edit"
	// ActionDelete permits soft-deleting or purging a task.
	ActionDelete Action = "Delete"
	// ActionUpload permits uploading new file attachments to a task.
	ActionUpload Action = "Upload"
	// ActionModerateComments permits deleting comments authored by other users.
	ActionModerateComments Action = "ModerateComments"
	// ActionModerateAttachments permits deleting attachments uploaded by other users.
	ActionModerateAttachments Action = "ModerateAttachments"
)

// ProjectRole defines the collaborator role within a project workspace.
type ProjectRole string

const (
	RoleOwner  ProjectRole = "owner"
	RoleEditor ProjectRole = "editor"
	RoleViewer ProjectRole = "viewer"
)

// Sentinel authorization errors for API policy mapping.
var (
	// ErrNotFound is returned when an unauthorized outsider or soft-deleted resource is accessed (anti-IDOR).
	ErrNotFound = errors.New("resource not found")
	// ErrForbidden is returned when a caller has read visibility but lacks the role required for the requested action.
	ErrForbidden = errors.New("insufficient permissions for this action")
)

// Subject represents the authenticated caller requesting access.
type Subject struct {
	UserID uuid.UUID
}

// Resource represents the target task and its associated context.
type Resource struct {
	TaskID         uuid.UUID
	ProjectID      *uuid.UUID
	ProjectDeleted bool
	CreatorID      uuid.UUID
	AssigneeID     *uuid.UUID
	MemberRole     *ProjectRole
}

// ParseRole converts a string into a ProjectRole pointer, returning nil if the role is unrecognized or empty.
func ParseRole(s string) *ProjectRole {
	switch ProjectRole(s) {
	case RoleOwner:
		r := RoleOwner
		return &r
	case RoleEditor:
		r := RoleEditor
		return &r
	case RoleViewer:
		r := RoleViewer
		return &r
	default:
		return nil
	}
}

// RolePtr returns a pointer to the given ProjectRole.
func RolePtr(r ProjectRole) *ProjectRole {
	return &r
}

// Can evaluates whether the subject is permitted to perform the action on the resource
// according to the permission matrix formalized in ADR-018.
// It is a pure, in-memory function with no I/O, database queries, or framework dependencies.
func Can(sub Subject, res Resource, action Action) bool {
	if sub.UserID == uuid.Nil {
		return false
	}

	// 1. Project Tasks (res.ProjectID != nil)
	// Security boundary is the project: caller's rights derive strictly from project membership.
	if res.ProjectID != nil {
		// If the parent project is soft-deleted, task is inaccessible (anti-IDOR).
		if res.ProjectDeleted {
			return false
		}

		// Outsider (caller has no active membership in the project).
		// Creator or assignee status outside of active project membership grants NO bypass rights.
		if res.MemberRole == nil {
			return false
		}

		switch *res.MemberRole {
		case RoleOwner:
			// Project owner has full permissions across all actions
			switch action {
			case ActionRead, ActionComment, ActionEdit, ActionDelete, ActionUpload,
				ActionModerateComments, ActionModerateAttachments:
				return true
			default:
				return false
			}

		case RoleEditor:
			// Editor can Read, Comment, Edit, Delete, and Upload.
			// Editor CANNOT moderate comments or attachments authored by other members.
			switch action {
			case ActionRead, ActionComment, ActionEdit, ActionDelete, ActionUpload:
				return true
			case ActionModerateComments, ActionModerateAttachments:
				return false
			default:
				return false
			}

		case RoleViewer:
			// Viewer can only Read and Comment.
			// Viewer CANNOT Edit, Delete, Upload, or Moderate.
			switch action {
			case ActionRead, ActionComment:
				return true
			case ActionEdit, ActionDelete, ActionUpload,
				ActionModerateComments, ActionModerateAttachments:
				return false
			default:
				return false
			}

		default:
			return false
		}
	}

	// 2. Personal Tasks (res.ProjectID == nil)
	// Task belongs directly to the individual creator.
	isCreator := sub.UserID == res.CreatorID
	isAssignee := res.AssigneeID != nil && sub.UserID == *res.AssigneeID

	if isCreator {
		// Personal task creator has full authority
		switch action {
		case ActionRead, ActionComment, ActionEdit, ActionDelete, ActionUpload,
			ActionModerateComments, ActionModerateAttachments:
			return true
		default:
			return false
		}
	}

	if isAssignee {
		// Personal task assignee can Read, Comment, and Upload.
		// Assignee CANNOT Edit task fields, Delete task, or Moderate.
		switch action {
		case ActionRead, ActionComment, ActionUpload:
			return true
		case ActionEdit, ActionDelete,
			ActionModerateComments, ActionModerateAttachments:
			return false
		default:
			return false
		}
	}

	// Outsider on personal task
	return false
}

// Authorize checks whether the subject is permitted to perform the action.
// If allowed, it returns nil.
// If disallowed, it enforces Anti-IDOR by returning:
// - ErrNotFound if the subject lacks Read visibility (or project is deleted/outsider)
// - ErrForbidden if the subject has Read visibility but lacks the specific action privilege.
func Authorize(sub Subject, res Resource, action Action) error {
	// Anti-IDOR: if caller cannot even Read the task, return ErrNotFound
	if !Can(sub, res, ActionRead) {
		return ErrNotFound
	}

	if !Can(sub, res, action) {
		return ErrForbidden
	}

	return nil
}

// CanModifyComment checks whether the subject can edit or delete a specific comment.
// A user can delete/modify their own comment as long as they hold Read access to the task.
// If not the author, they require ActionModerateComments permission.
func CanModifyComment(sub Subject, res Resource, commentAuthorID uuid.UUID) bool {
	if !Can(sub, res, ActionRead) {
		return false
	}
	if sub.UserID != uuid.Nil && sub.UserID == commentAuthorID {
		return true
	}
	return Can(sub, res, ActionModerateComments)
}

// CanDeleteAttachment checks whether the subject can delete a specific attachment.
// A user can delete their own attachment as long as they hold Read access to the task.
// If not the uploader, they require ActionModerateAttachments permission.
func CanDeleteAttachment(sub Subject, res Resource, attachmentUploaderID uuid.UUID) bool {
	if !Can(sub, res, ActionRead) {
		return false
	}
	if sub.UserID != uuid.Nil && sub.UserID == attachmentUploaderID {
		return true
	}
	return Can(sub, res, ActionModerateAttachments)
}

// AuthorizeModifyComment checks whether the subject can edit/delete a comment, returning:
// - nil if allowed
// - ErrNotFound if the subject lacks Read access to the task
// - ErrForbidden if the subject has Read access but lacks authority over this specific comment
func AuthorizeModifyComment(sub Subject, res Resource, commentAuthorID uuid.UUID) error {
	if !Can(sub, res, ActionRead) {
		return ErrNotFound
	}
	if sub.UserID != uuid.Nil && sub.UserID == commentAuthorID {
		return nil
	}
	if Can(sub, res, ActionModerateComments) {
		return nil
	}
	return ErrForbidden
}

// AuthorizeDeleteAttachment checks whether the subject can delete an attachment, returning:
// - nil if allowed
// - ErrNotFound if the subject lacks Read access to the task
// - ErrForbidden if the subject has Read access but lacks authority over this specific attachment
func AuthorizeDeleteAttachment(sub Subject, res Resource, attachmentUploaderID uuid.UUID) error {
	if !Can(sub, res, ActionRead) {
		return ErrNotFound
	}
	if sub.UserID != uuid.Nil && sub.UserID == attachmentUploaderID {
		return nil
	}
	if Can(sub, res, ActionModerateAttachments) {
		return nil
	}
	return ErrForbidden
}
