package hasher

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidPassword indicates mismatch between password and hash.
	ErrInvalidPassword = errors.New("invalid password")
	// ErrPasswordTooLong indicates password exceeds bcrypt 72 byte limit.
	ErrPasswordTooLong = errors.New("password exceeds maximum allowed length of 72 bytes")
)

// PasswordHasher defines methods for hashing and verifying passwords.
type PasswordHasher interface {
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error
}

// BcryptHasher implements PasswordHasher using bcrypt.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher creates a new BcryptHasher with given cost.
// If cost <= 0, bcrypt.DefaultCost (10) is used. Recommended production cost is 12.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

// HashPassword generates a bcrypt hash of the given plain password.
func (h *BcryptHasher) HashPassword(password string) (string, error) {
	if len(password) > 72 {
		return "", ErrPasswordTooLong
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// ComparePassword compares a bcrypt hashed password with its possible plaintext equivalent.
func (h *BcryptHasher) ComparePassword(hashedPassword, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidPassword
		}
		return fmt.Errorf("failed to compare password hash: %w", err)
	}
	return nil
}
