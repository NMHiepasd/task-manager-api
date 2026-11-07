package service

import (
	"context"
	"errors"
	"fmt"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UserService defines operations for managing user profiles.
type UserService interface {
	GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error)
}

type userService struct {
	repo  repository.Querier
	cache cache.Cache
}

// NewUserService constructs a new UserService instance.
func NewUserService(repo repository.Querier, cacheStore cache.Cache) UserService {
	if cacheStore == nil {
		cacheStore = cache.NewNoopCache()
	}
	return &userService{repo: repo, cache: cacheStore}
}

// GetProfile retrieves user profile by ID using cache-aside.
func (s *userService) GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error) {
	cacheKey := fmt.Sprintf("user:%s:profile", userID)
	var resp UserResponse

	err := s.cache.Remember(ctx, cacheKey, 1*time.Hour, &resp, func(fetchCtx context.Context) (any, error) {
		user, err := s.repo.GetUserByID(fetchCtx, userID)
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
	})
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
