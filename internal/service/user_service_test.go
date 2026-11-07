package service

import (
	"context"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestUserService_GetProfile_Success(t *testing.T) {
	userID := uuid.New()
	mock := &mockQuerier{
		getUserByIDFn: func(_ context.Context, id uuid.UUID) (repository.User, error) {
			require.Equal(t, userID, id)
			return repository.User{
				ID:        userID,
				Email:     "user@example.com",
				FullName:  "Profile User",
				Role:      "user",
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
	}

	userService := NewUserService(mock, nil)
	profile, err := userService.GetProfile(context.Background(), userID)

	require.NoError(t, err)
	require.NotNil(t, profile)
	require.Equal(t, userID, profile.ID)
	require.Equal(t, "user@example.com", profile.Email)
	require.Equal(t, "Profile User", profile.FullName)
}

func TestUserService_GetProfile_NotFound(t *testing.T) {
	mock := &mockQuerier{
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (repository.User, error) {
			return repository.User{}, pgx.ErrNoRows
		},
	}

	userService := NewUserService(mock, nil)
	profile, err := userService.GetProfile(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrUserNotFound)
	require.Nil(t, profile)
}
