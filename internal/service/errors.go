package service

import (
	"task-manager-api/internal/domain"
)

var (
	ErrEmailAlreadyExists     = domain.ErrEmailAlreadyExists
	ErrInvalidCredentials     = domain.ErrInvalidCredentials
	ErrUserNotFound           = domain.ErrUserNotFound
	ErrInvalidToken           = domain.ErrInvalidToken
	ErrTokenExpired           = domain.ErrTokenExpired
	ErrTokenRevoked           = domain.ErrTokenRevoked
	ErrTokenReuseDetected     = domain.ErrTokenReuseDetected
	ErrPasswordTooLong        = domain.ErrPasswordTooLong
	ErrProjectNotFound        = domain.ErrProjectNotFound
	ErrOptimisticLockConflict = domain.ErrOptimisticLockConflict
	ErrForbidden              = domain.ErrForbidden
	ErrTaskNotFound           = domain.ErrTaskNotFound
	ErrInvalidTaskStatus      = domain.ErrInvalidTaskStatus
	ErrInvalidTaskPriority    = domain.ErrInvalidTaskPriority
	ErrTagNotFound            = domain.ErrTagNotFound
)
