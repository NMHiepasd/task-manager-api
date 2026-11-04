package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	// ErrInvalidToken indicates token signature or claims could not be verified.
	ErrInvalidToken = errors.New("token is invalid")
	// ErrExpiredToken indicates token validity period has lapsed.
	ErrExpiredToken = errors.New("token has expired")
	// ErrSecretTooShort indicates secret key length is less than 32 characters.
	ErrSecretTooShort = errors.New("secret key must be at least 32 characters long")
)

// UserClaims holds custom and standard JWT claims.
type UserClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// Maker is an interface for managing authentication tokens.
type Maker interface {
	CreateAccessToken(userID uuid.UUID, email string, role string, duration time.Duration) (string, *UserClaims, error)
	VerifyToken(tokenString string) (*UserClaims, error)
	GenerateRefreshToken() (plainToken string, tokenHash string, err error)
	HashRefreshToken(plainToken string) string
}

// JWTMaker implements Maker using HMAC-SHA256.
type JWTMaker struct {
	secretKey []byte
}

// NewJWTMaker creates a new JWTMaker with the provided secret key.
func NewJWTMaker(secretKey string) (*JWTMaker, error) {
	if len(secretKey) < 32 {
		return nil, ErrSecretTooShort
	}
	return &JWTMaker{secretKey: []byte(secretKey)}, nil
}

// CreateAccessToken creates a new signed JWT access token for a specific user.
func (m *JWTMaker) CreateAccessToken(userID uuid.UUID, email string, role string, duration time.Duration) (string, *UserClaims, error) {
	tokenID, err := uuid.NewV7()
	if err != nil {
		tokenID = uuid.New()
	}

	now := time.Now().UTC()
	claims := &UserClaims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID.String(),
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, claims, nil
}

// VerifyToken checks if the token is valid and parses claims.
func (m *JWTMaker) VerifyToken(tokenString string) (*UserClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, token.Header["alg"])
		}
		return m.secretKey, nil
	}

	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, keyFunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GenerateRefreshToken generates an opaque cryptographically secure 32-byte token and its SHA-256 hash.
func (m *JWTMaker) GenerateRefreshToken() (string, string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes for refresh token: %w", err)
	}

	plainToken := hex.EncodeToString(randomBytes)
	tokenHash := m.HashRefreshToken(plainToken)

	return plainToken, tokenHash, nil
}

// HashRefreshToken calculates SHA-256 hash hex string of the plain refresh token.
func (m *JWTMaker) HashRefreshToken(plainToken string) string {
	hash := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(hash[:])
}
