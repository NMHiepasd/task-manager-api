package service

import (
	"context"
	"errors"
	"fmt"
	"task-manager-api/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UserService defines operations for managing user profiles.
type UserService interface {
	GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error)
}

type userService struct {
	repo repository.Querier
}

// NewUserService constructs a new UserService instance.
func NewUserService(repo repository.Querier) UserService {
	return &userService{repo: repo}
}

// GetProfile retrieves user profile by ID.
func (s *userService) GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to retrieve user profile: %w", err)
	}

	return &UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		FullName:  user.FullName,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Time,
	}, nil
}
