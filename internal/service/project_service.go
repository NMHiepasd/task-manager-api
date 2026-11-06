package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"task-manager-api/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateProjectInput holds payload for creating a new project.
type CreateProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// UpdateProjectInput holds payload for updating an existing project.
type UpdateProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Version     int32  `json:"version"`
}

// ProjectResponse represents public project data.
type ProjectResponse struct {
	ID          uuid.UUID `json:"id"`
	OwnerID     uuid.UUID `json:"owner_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	Version     int32     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PaginationParams specifies pagination offset/limit.
type PaginationParams struct {
	Page  int32 `json:"page"`
	Limit int32 `json:"limit"`
}

// PaginatedProjectsResponse represents paginated project results.
type PaginatedProjectsResponse struct {
	Items      []ProjectResponse `json:"items"`
	TotalItems int64             `json:"total_items"`
	Page       int32             `json:"page"`
	Limit      int32             `json:"limit"`
	TotalPages int32             `json:"total_pages"`
}

// ProjectService defines business logic for managing projects.
type ProjectService interface {
	CreateProject(ctx context.Context, ownerID uuid.UUID, input CreateProjectInput) (*ProjectResponse, error)
	GetProjectByID(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) (*ProjectResponse, error)
	ListProjects(ctx context.Context, ownerID uuid.UUID, pagination PaginationParams) (*PaginatedProjectsResponse, error)
	UpdateProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input UpdateProjectInput) (*ProjectResponse, error)
	DeleteProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error
}

type projectService struct {
	repo repository.Querier
}

// NewProjectService constructs a new ProjectService.
func NewProjectService(repo repository.Querier) ProjectService {
	return &projectService{repo: repo}
}

// CreateProject creates a project for the authenticated owner.
func (s *projectService) CreateProject(ctx context.Context, ownerID uuid.UUID, input CreateProjectInput) (*ProjectResponse, error) {
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#3b82f6"
	}

	project, err := s.repo.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		Color:       color,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return s.mapProject(project), nil
}

// GetProjectByID retrieves a single project, verifying ownership.
func (s *projectService) GetProjectByID(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) (*ProjectResponse, error) {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to query project: %w", err)
	}

	if project.OwnerID != ownerID {
		return nil, ErrForbidden
	}

	return s.mapProject(project), nil
}

// ListProjects retrieves paginated projects owned by the user.
func (s *projectService) ListProjects(ctx context.Context, ownerID uuid.UUID, pagination PaginationParams) (*PaginatedProjectsResponse, error) {
	page := pagination.Page
	if page <= 0 {
		page = 1
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	total, err := s.repo.CountProjectsByOwner(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to count projects: %w", err)
	}

	projects, err := s.repo.ListProjectsByOwner(ctx, repository.ListProjectsByOwnerParams{
		OwnerID: ownerID,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	items := make([]ProjectResponse, len(projects))
	for i, p := range projects {
		items[i] = *s.mapProject(p)
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))

	return &PaginatedProjectsResponse{
		Items:      items,
		TotalItems: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// UpdateProject updates project details with optimistic concurrency check (version matching).
func (s *projectService) UpdateProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input UpdateProjectInput) (*ProjectResponse, error) {
	// 1. Verify existence and ownership
	existing, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to check existing project: %w", err)
	}

	if existing.OwnerID != ownerID {
		return nil, ErrForbidden
	}

	// 2. Check optimistic lock version
	if existing.Version != input.Version {
		return nil, ErrOptimisticLockConflict
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = existing.Name
	}

	description := strings.TrimSpace(input.Description)
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = existing.Color
	}

	updated, err := s.repo.UpdateProject(ctx, repository.UpdateProjectParams{
		ID:          projectID,
		Name:        name,
		Description: description,
		Color:       color,
		OwnerID:     ownerID,
		Version:     input.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOptimisticLockConflict
		}
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return s.mapProject(updated), nil
}

// DeleteProject soft-deletes the project owned by user.
func (s *projectService) DeleteProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error {
	existing, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("failed to check project before deletion: %w", err)
	}

	if existing.OwnerID != ownerID {
		return ErrForbidden
	}

	if err := s.repo.SoftDeleteProject(ctx, repository.SoftDeleteProjectParams{
		ID:      projectID,
		OwnerID: ownerID,
	}); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}

func (s *projectService) mapProject(p repository.Project) *ProjectResponse {
	return &ProjectResponse{
		ID:          p.ID,
		OwnerID:     p.OwnerID,
		Name:        p.Name,
		Description: p.Description,
		Color:       p.Color,
		Version:     p.Version,
		CreatedAt:   p.CreatedAt.Time,
		UpdatedAt:   p.UpdatedAt.Time,
	}
}
