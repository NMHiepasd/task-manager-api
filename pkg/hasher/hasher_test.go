package hasher

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher_HashAndCompare(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost) // Use MinCost for fast tests

	password := "SecretP@ssw0rd123!"

	hashed, err := h.HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashed)
	require.NotEqual(t, password, hashed)

	// Valid password
	err = h.ComparePassword(hashed, password)
	require.NoError(t, err)

	// Wrong password
	err = h.ComparePassword(hashed, "WrongPassword")
	require.ErrorIs(t, err, ErrInvalidPassword)
}

func TestBcryptHasher_PasswordTooLong(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)

	longPassword := strings.Repeat("a", 73)
	_, err := h.HashPassword(longPassword)
	require.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestBcryptHasher_DefaultCostFallback(t *testing.T) {
	h := NewBcryptHasher(-1)
	require.Equal(t, bcrypt.DefaultCost, h.cost)

	h2 := NewBcryptHasher(99)
	require.Equal(t, bcrypt.DefaultCost, h2.cost)
}
