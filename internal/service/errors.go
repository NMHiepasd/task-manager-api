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
)
