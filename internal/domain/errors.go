package domain

import (
	"errors"
	"fmt"
)

var (
	// Root category errors
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrValidation   = errors.New("validation failed")
	ErrInternal     = errors.New("internal server error")

	// Specific domain errors
	ErrEmailAlreadyExists         = fmt.Errorf("%w: email is already registered", ErrConflict)
	ErrInvalidCredentials         = fmt.Errorf("%w: invalid email or password", ErrUnauthorized)
	ErrUserNotFound               = fmt.Errorf("%w: user not found", ErrNotFound)
	ErrInvalidToken               = fmt.Errorf("%w: token is invalid", ErrUnauthorized)
	ErrTokenExpired               = fmt.Errorf("%w: token has expired", ErrUnauthorized)
	ErrTokenRevoked               = fmt.Errorf("%w: token has been revoked", ErrUnauthorized)
	ErrTokenReuseDetected         = fmt.Errorf("%w: token reuse detected; all sessions revoked", ErrUnauthorized)
	ErrPasswordTooLong            = fmt.Errorf("%w: password exceeds maximum allowed length of 72 bytes", ErrValidation)
	ErrProjectNotFound            = fmt.Errorf("%w: project not found", ErrNotFound)
	ErrProjectMemberNotFound      = fmt.Errorf("%w: project member not found", ErrNotFound)
	ErrProjectMemberAlreadyExists = fmt.Errorf("%w: user is already a member of this project", ErrConflict)
	ErrCannotRemoveProjectOwner   = fmt.Errorf("%w: cannot remove project owner", ErrValidation)
	ErrCannotModifyProjectOwner   = fmt.Errorf("%w: cannot modify project owner role", ErrValidation)
	ErrInsufficientProjectRole    = fmt.Errorf("%w: you do not have permission to perform this action in the project", ErrForbidden)
	ErrInvalidProjectRole         = fmt.Errorf("%w: invalid project member role; allowed: editor, viewer", ErrValidation)
	ErrOptimisticLockConflict     = fmt.Errorf("%w: resource has been modified by another request; please reload and try again", ErrConflict)
	ErrTaskNotFound               = fmt.Errorf("%w: task not found", ErrNotFound)
	ErrInvalidTaskStatus          = fmt.Errorf("%w: invalid task status; allowed: todo, in_progress, completed, cancelled", ErrValidation)
	ErrInvalidTaskPriority        = fmt.Errorf("%w: invalid task priority; allowed: low, medium, high, urgent", ErrValidation)
	ErrTagNotFound                = fmt.Errorf("%w: tag not found", ErrNotFound)
	ErrPayloadTooLarge            = fmt.Errorf("%w: request payload exceeds size limit", ErrValidation)
	ErrRateLimitExceeded          = errors.New("rate limit exceeded; please wait before retrying")
	ErrTimeout                    = errors.New("request processing timed out")
	ErrIdempotencyConflict        = fmt.Errorf("%w: concurrent request with the same idempotency key is currently processing", ErrConflict)
	ErrIdempotencyMismatch        = fmt.Errorf("%w: idempotency key was previously used with different request parameters", ErrConflict)
)

// InvalidParam describes a single invalid request field for RFC 9457.
type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ValidationErrors is an error type containing field-level validation failures.
type ValidationErrors struct {
	Message string         `json:"message"`
	Params  []InvalidParam `json:"params"`
}

func (v ValidationErrors) Error() string {
	if v.Message != "" {
		return v.Message
	}
	return "input validation failed"
}

func (v ValidationErrors) Unwrap() error {
	return ErrValidation
}
