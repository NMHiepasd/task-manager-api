package service

import (
	"task-manager-api/internal/domain"
)

var (
	ErrEmailAlreadyExists         = domain.ErrEmailAlreadyExists
	ErrInvalidCredentials         = domain.ErrInvalidCredentials
	ErrUserNotFound               = domain.ErrUserNotFound
	ErrInvalidToken               = domain.ErrInvalidToken
	ErrTokenExpired               = domain.ErrTokenExpired
	ErrTokenRevoked               = domain.ErrTokenRevoked
	ErrTokenReuseDetected         = domain.ErrTokenReuseDetected
	ErrPasswordTooLong            = domain.ErrPasswordTooLong
	ErrProjectNotFound            = domain.ErrProjectNotFound
	ErrProjectMemberNotFound      = domain.ErrProjectMemberNotFound
	ErrProjectMemberAlreadyExists = domain.ErrProjectMemberAlreadyExists
	ErrCannotRemoveProjectOwner   = domain.ErrCannotRemoveProjectOwner
	ErrCannotModifyProjectOwner   = domain.ErrCannotModifyProjectOwner
	ErrInsufficientProjectRole    = domain.ErrInsufficientProjectRole
	ErrInvalidProjectRole         = domain.ErrInvalidProjectRole
	ErrOptimisticLockConflict     = domain.ErrOptimisticLockConflict
	ErrForbidden                  = domain.ErrForbidden
	ErrTaskNotFound               = domain.ErrTaskNotFound
	ErrInvalidTaskStatus          = domain.ErrInvalidTaskStatus
	ErrInvalidTaskPriority        = domain.ErrInvalidTaskPriority
	ErrAssigneeNotFound           = domain.ErrAssigneeNotFound
	ErrAssigneeNotInProject       = domain.ErrAssigneeNotInProject
	ErrTagNotFound                = domain.ErrTagNotFound
	ErrCommentNotFound            = domain.ErrCommentNotFound
	ErrEmptyCommentContent        = domain.ErrEmptyCommentContent
	ErrAttachmentNotFound         = domain.ErrAttachmentNotFound
	ErrFileTooLarge               = domain.ErrFileTooLarge
	ErrInvalidFileType            = domain.ErrInvalidFileType
	ErrEmptyFile                  = domain.ErrEmptyFile
	ErrWebhookNotFound            = domain.ErrWebhookNotFound
	ErrInvalidWebhookURL          = domain.ErrInvalidWebhookURL
	ErrWebhookSSRF                = domain.ErrWebhookSSRF
	ErrInvalidWebhookEvents       = domain.ErrInvalidWebhookEvents
)
