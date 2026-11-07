package service

import (
	"context"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

type mockTagQuerier struct {
	mockQuerier
	createTagFn      func(ctx context.Context, arg repository.CreateTagParams) (repository.Tag, error)
	listTagsByUserFn func(ctx context.Context, userID uuid.UUID) ([]repository.Tag, error)
	deleteTagFn      func(ctx context.Context, arg repository.DeleteTagParams) error
}

func (m *mockTagQuerier) CreateTag(ctx context.Context, arg repository.CreateTagParams) (repository.Tag, error) {
	if m.createTagFn != nil {
		return m.createTagFn(ctx, arg)
	}
	return repository.Tag{}, nil
}

func (m *mockTagQuerier) ListTagsByUser(ctx context.Context, userID uuid.UUID) ([]repository.Tag, error) {
	if m.listTagsByUserFn != nil {
		return m.listTagsByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockTagQuerier) DeleteTag(ctx context.Context, arg repository.DeleteTagParams) error {
	if m.deleteTagFn != nil {
		return m.deleteTagFn(ctx, arg)
	}
	return nil
}

func TestTagService_CreateTag(t *testing.T) {
	userID := uuid.New()
	tagID := uuid.New()

	mock := &mockTagQuerier{
		createTagFn: func(_ context.Context, arg repository.CreateTagParams) (repository.Tag, error) {
			require.Equal(t, userID, arg.UserID)
			require.Equal(t, "Backend", arg.Name)
			require.Equal(t, "#10b981", arg.Color)
			return repository.Tag{
				ID:        tagID,
				UserID:    arg.UserID,
				Name:      arg.Name,
				Color:     arg.Color,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
	}

	svc := NewTagService(mock, nil)
	res, err := svc.CreateTag(context.Background(), userID, CreateTagInput{
		Name: "Backend",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "Backend", res.Name)
	require.Equal(t, "#10b981", res.Color)
}

func TestTagService_ListTags(t *testing.T) {
	userID := uuid.New()

	mock := &mockTagQuerier{
		listTagsByUserFn: func(_ context.Context, uID uuid.UUID) ([]repository.Tag, error) {
			require.Equal(t, userID, uID)
			return []repository.Tag{
				{ID: uuid.New(), UserID: userID, Name: "Tag1", Color: "#ff0000"},
				{ID: uuid.New(), UserID: userID, Name: "Tag2", Color: "#00ff00"},
			}, nil
		},
	}

	svc := NewTagService(mock, nil)
	tags, err := svc.ListTags(context.Background(), userID)

	require.NoError(t, err)
	require.Len(t, tags, 2)
}

func TestTagService_DeleteTag(t *testing.T) {
	userID := uuid.New()
	tagID := uuid.New()
	deleted := false

	mock := &mockTagQuerier{
		deleteTagFn: func(_ context.Context, arg repository.DeleteTagParams) error {
			require.Equal(t, tagID, arg.ID)
			require.Equal(t, userID, arg.UserID)
			deleted = true
			return nil
		},
	}

	svc := NewTagService(mock, nil)
	err := svc.DeleteTag(context.Background(), userID, tagID)
	require.NoError(t, err)
	require.True(t, deleted)
}
