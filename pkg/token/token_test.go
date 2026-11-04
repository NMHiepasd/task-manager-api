package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const testSecretKey = "12345678901234567890123456789012" // 32 chars

func TestJWTMaker_CreateAndVerifyToken(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	email := "user@example.com"
	role := "user"
	duration := 15 * time.Minute

	tokenString, claims, err := maker.CreateAccessToken(userID, email, role, duration)
	require.NoError(t, err)
	require.NotEmpty(t, tokenString)
	require.NotNil(t, claims)

	// Verify token
	verifiedClaims, err := maker.VerifyToken(tokenString)
	require.NoError(t, err)
	require.NotNil(t, verifiedClaims)

	require.Equal(t, userID, verifiedClaims.UserID)
	require.Equal(t, email, verifiedClaims.Email)
	require.Equal(t, role, verifiedClaims.Role)
	require.WithinDuration(t, time.Now().Add(duration), verifiedClaims.ExpiresAt.Time, 5*time.Second)
}

func TestJWTMaker_ExpiredToken(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	email := "user@example.com"
	role := "user"

	// Create expired token
	tokenString, _, err := maker.CreateAccessToken(userID, email, role, -time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	verifiedClaims, err := maker.VerifyToken(tokenString)
	require.ErrorIs(t, err, ErrExpiredToken)
	require.Nil(t, verifiedClaims)
}

func TestJWTMaker_InvalidSignature(t *testing.T) {
	maker1, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	maker2, err := NewJWTMaker("different_secret_key_that_is_32_bytes_long!!")
	require.NoError(t, err)

	userID := uuid.New()
	email := "user@example.com"

	tokenString, _, err := maker1.CreateAccessToken(userID, email, "user", 15*time.Minute)
	require.NoError(t, err)

	// Verify with wrong secret
	verifiedClaims, err := maker2.VerifyToken(tokenString)
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Nil(t, verifiedClaims)
}

func TestJWTMaker_InvalidNoneAlgorithm(t *testing.T) {
	userID := uuid.New()
	claims := &UserClaims{
		UserID: userID,
		Email:  "hacker@example.com",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}

	// Sign with 'none' alg
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	verifiedClaims, err := maker.VerifyToken(tokenString)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Nil(t, verifiedClaims)
}

func TestJWTMaker_SecretKeyTooShort(t *testing.T) {
	_, err := NewJWTMaker("too-short")
	require.ErrorIs(t, err, ErrSecretTooShort)
}

func TestJWTMaker_RefreshTokenGenerationAndHash(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	plainToken1, hash1, err := maker.GenerateRefreshToken()
	require.NoError(t, err)
	require.Len(t, plainToken1, 64) // 32 bytes hex encoded = 64 hex chars
	require.Len(t, hash1, 64)       // SHA256 hex encoded = 64 hex chars

	// Hashing plain token again must yield identical hash
	hash2 := maker.HashRefreshToken(plainToken1)
	require.Equal(t, hash1, hash2)

	// Another random token must be different
	plainToken2, hash3, err := maker.GenerateRefreshToken()
	require.NoError(t, err)
	require.NotEqual(t, plainToken1, plainToken2)
	require.NotEqual(t, hash1, hash3)
}
