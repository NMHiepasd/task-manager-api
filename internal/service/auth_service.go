package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/hasher"
	"task-manager-api/pkg/token"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RegisterInput contains user registration data.
type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// LoginInput contains credentials for authentication.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse represents public profile data.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// TokenResponse represents JWT authentication result.
type TokenResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"` // In seconds
	User         UserResponse `json:"user"`
}

// AuthService defines operations for authentication and token lifecycle.
type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (*TokenResponse, error)
	Login(ctx context.Context, input LoginInput) (*TokenResponse, error)
	RefreshToken(ctx context.Context, plainRefreshToken string) (*TokenResponse, error)
	Logout(ctx context.Context, plainRefreshToken string) error
}

type authService struct {
	repo                 repository.Querier
	hasher               hasher.PasswordHasher
	tokenMaker           token.Maker
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
}

// NewAuthService constructs a new AuthService instance.
func NewAuthService(
	repo repository.Querier,
	hasher hasher.PasswordHasher,
	tokenMaker token.Maker,
	accessTokenDuration time.Duration,
	refreshTokenDuration time.Duration,
) AuthService {
	return &authService{
		repo:                 repo,
		hasher:               hasher,
		tokenMaker:           tokenMaker,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
	}
}

// Register registers a new user, hashes password, and issues tokens.
func (s *authService) Register(ctx context.Context, input RegisterInput) (*TokenResponse, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))

	// Check if user already exists
	existingUser, err := s.repo.GetUserByEmail(ctx, email)
	if err == nil && existingUser.ID != uuid.Nil {
		return nil, ErrEmailAlreadyExists
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}

	// Hash password
	hashedPassword, err := s.hasher.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	// Persist user
	user, err := s.repo.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: hashedPassword,
		FullName:     strings.TrimSpace(input.FullName),
		Role:         "user",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return s.issueTokenPair(ctx, user)
}

// Login verifies credentials and issues a fresh token pair.
func (s *authService) Login(ctx context.Context, input LoginInput) (*TokenResponse, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to retrieve user: %w", err)
	}

	if err := s.hasher.ComparePassword(user.PasswordHash, input.Password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokenPair(ctx, user)
}

// RefreshToken validates an active refresh token, revokes it, and issues a new pair (Rotation).
// If a revoked token is reused, all tokens for that user are immediately invalidated (Reuse Detection).
func (s *authService) RefreshToken(ctx context.Context, plainRefreshToken string) (*TokenResponse, error) {
	tokenHash := s.tokenMaker.HashRefreshToken(plainRefreshToken)

	rt, err := s.repo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed to query refresh token: %w", err)
	}

	// Reuse detection: already revoked token was submitted
	if rt.RevokedAt.Valid {
		_ = s.repo.RevokeAllUserRefreshTokens(ctx, rt.UserID)
		return nil, ErrTokenReuseDetected
	}

	// Check expiration
	if rt.ExpiresAt.Time.Before(time.Now().UTC()) {
		_ = s.repo.RevokeRefreshToken(ctx, rt.TokenHash)
		return nil, ErrTokenExpired
	}

	// Revoke current token (Token Rotation)
	if err := s.repo.RevokeRefreshToken(ctx, rt.TokenHash); err != nil {
		return nil, fmt.Errorf("failed to rotate refresh token: %w", err)
	}

	// Fetch active user
	user, err := s.repo.GetUserByID(ctx, rt.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to retrieve user: %w", err)
	}

	return s.issueTokenPair(ctx, user)
}

// Logout revokes the given refresh token.
func (s *authService) Logout(ctx context.Context, plainRefreshToken string) error {
	tokenHash := s.tokenMaker.HashRefreshToken(plainRefreshToken)
	return s.repo.RevokeRefreshToken(ctx, tokenHash)
}

// issueTokenPair generates a new access token and refresh token for the user.
func (s *authService) issueTokenPair(ctx context.Context, user repository.User) (*TokenResponse, error) {
	accessToken, _, err := s.tokenMaker.CreateAccessToken(user.ID, user.Email, user.Role, s.accessTokenDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	plainRefreshToken, tokenHash, err := s.tokenMaker.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.refreshTokenDuration)
	_, err = s.repo.CreateRefreshToken(ctx, repository.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{
			Time:  expiresAt,
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist refresh token: %w", err)
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTokenDuration.Seconds()),
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Time,
		},
	}, nil
}
