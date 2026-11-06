package service

import (
	"context"
	"fmt"
	"strings"
	"task-manager-api/internal/repository"
	"time"

	"github.com/google/uuid"
)

// CreateTagInput specifies parameters for creating or updating a tag.
type CreateTagInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// TagResponse represents public tag data.
type TagResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// TagService defines business logic for managing tags.
type TagService interface {
	CreateTag(ctx context.Context, userID uuid.UUID, input CreateTagInput) (*TagResponse, error)
	ListTags(ctx context.Context, userID uuid.UUID) ([]TagResponse, error)
	DeleteTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error
}

type tagService struct {
	repo repository.Querier
}

// NewTagService constructs a new TagService instance.
func NewTagService(repo repository.Querier) TagService {
	return &tagService{repo: repo}
}

// CreateTag creates or upserts a tag for the user.
func (s *tagService) CreateTag(ctx context.Context, userID uuid.UUID, input CreateTagInput) (*TagResponse, error) {
	name := strings.TrimSpace(input.Name)
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#10b981"
	}

	tag, err := s.repo.CreateTag(ctx, repository.CreateTagParams{
		UserID: userID,
		Name:   name,
		Color:  color,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return s.mapTag(tag), nil
}

// ListTags lists all tags owned by the user.
func (s *tagService) ListTags(ctx context.Context, userID uuid.UUID) ([]TagResponse, error) {
	tags, err := s.repo.ListTagsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	res := make([]TagResponse, len(tags))
	for i, t := range tags {
		res[i] = *s.mapTag(t)
	}
	return res, nil
}

// DeleteTag deletes a tag owned by the user.
func (s *tagService) DeleteTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	if err := s.repo.DeleteTag(ctx, repository.DeleteTagParams{
		ID:     tagID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}
	return nil
}

func (s *tagService) mapTag(t repository.Tag) *TagResponse {
	return &TagResponse{
		ID:        t.ID,
		UserID:    t.UserID,
		Name:      t.Name,
		Color:     t.Color,
		CreatedAt: t.CreatedAt.Time,
	}
}
