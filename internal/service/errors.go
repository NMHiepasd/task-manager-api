package service

import (
	"errors"
	"task-manager-api/pkg/hasher"
)

var (
	// ErrEmailAlreadyExists indicates registration with an already taken email.
	ErrEmailAlreadyExists = errors.New("email is already registered")
	// ErrInvalidCredentials indicates wrong email or password.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUserNotFound indicates the target user account does not exist or has been deleted.
	ErrUserNotFound = errors.New("user not found")
	// ErrInvalidToken indicates the token is malformed, not found, or invalid.
	ErrInvalidToken = errors.New("token is invalid")
	// ErrTokenExpired indicates the token validity period has ended.
	ErrTokenExpired = errors.New("token has expired")
	// ErrTokenRevoked indicates the token has been manually revoked.
	ErrTokenRevoked = errors.New("token has been revoked")
	// ErrTokenReuseDetected indicates a revoked token was attempted to be reused (potential theft).
	ErrTokenReuseDetected = errors.New("refresh token reuse detected; all sessions revoked for security")
	// ErrPasswordTooLong indicates password length exceeded allowed bcrypt size.
	ErrPasswordTooLong = hasher.ErrPasswordTooLong
	// ErrProjectNotFound indicates the requested project does not exist or has been deleted.
	ErrProjectNotFound = errors.New("project not found")
	// ErrOptimisticLockConflict indicates the resource version does not match the database version.
	ErrOptimisticLockConflict = errors.New("resource has been modified by another request; please reload and try again")
	// ErrForbidden indicates the authenticated user does not own or have permission to access the resource.
	ErrForbidden = errors.New("forbidden: you do not have permission to perform this action")
	// ErrTaskNotFound indicates the requested task does not exist or has been deleted.
	ErrTaskNotFound = errors.New("task not found")
	// ErrInvalidTaskStatus indicates the status string is not among the allowed enum values.
	ErrInvalidTaskStatus = errors.New("invalid task status; allowed values: todo, in_progress, completed, cancelled")
	// ErrInvalidTaskPriority indicates the priority string is not among the allowed enum values.
	ErrInvalidTaskPriority = errors.New("invalid task priority; allowed values: low, medium, high, urgent")
	// ErrTagNotFound indicates the tag does not exist.
	ErrTagNotFound = errors.New("tag not found")
)
