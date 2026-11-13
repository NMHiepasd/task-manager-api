package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/pagination"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	Role        string    `json:"role,omitempty"`
	Version     int32     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectMemberResponse represents project collaborator details.
type ProjectMemberResponse struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddProjectMemberInput specifies payload for adding a collaborator.
type AddProjectMemberInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UpdateProjectMemberRoleInput specifies payload for updating a member's role.
type UpdateProjectMemberRoleInput struct {
	Role string `json:"role"`
}

// PaginationParams specifies pagination offset/limit or keyset cursor.
type PaginationParams struct {
	Cursor string `json:"cursor"`
	Page   int32  `json:"page"`
	Limit  int32  `json:"limit"`
}

// PaginatedProjectsResponse represents paginated project results.
type PaginatedProjectsResponse struct {
	Items      []ProjectResponse `json:"items"`
	TotalItems int64             `json:"total_items"`
	Page       int32             `json:"page,omitempty"`
	Limit      int32             `json:"limit"`
	TotalPages int32             `json:"total_pages,omitempty"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

// ProjectService defines business logic for managing projects and collaborators.
type ProjectService interface {
	CreateProject(ctx context.Context, ownerID uuid.UUID, input CreateProjectInput) (*ProjectResponse, error)
	GetProjectByID(ctx context.Context, userID uuid.UUID, projectID uuid.UUID) (*ProjectResponse, error)
	ListProjects(ctx context.Context, userID uuid.UUID, pagination PaginationParams) (*PaginatedProjectsResponse, error)
	UpdateProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input UpdateProjectInput) (*ProjectResponse, error)
	DeleteProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error

	// Member management
	AddMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, input AddProjectMemberInput) (*ProjectMemberResponse, error)
	ListMembers(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID) ([]ProjectMemberResponse, error)
	UpdateMemberRole(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID, input UpdateProjectMemberRoleInput) (*ProjectMemberResponse, error)
	RemoveMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID) error
}

type projectService struct {
	repo  repository.Querier
	cache cache.Cache
}

// NewProjectService constructs a new ProjectService.
func NewProjectService(repo repository.Querier, cacheStore cache.Cache) ProjectService {
	if cacheStore == nil {
		cacheStore = cache.NewNoopCache()
	}
	return &projectService{repo: repo, cache: cacheStore}
}

// CreateProject creates a project for the authenticated owner and registers owner membership.
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

	// Register owner in project_members
	_, _ = s.repo.AddProjectMember(ctx, repository.AddProjectMemberParams{
		ProjectID: project.ID,
		UserID:    ownerID,
		Role:      "owner",
	})

	res := s.mapProject(project)
	res.Role = "owner"
	return res, nil
}

// GetProjectByID retrieves a single project, verifying access permissions (owner or collaborator).
func (s *projectService) GetProjectByID(ctx context.Context, userID uuid.UUID, projectID uuid.UUID) (*ProjectResponse, error) {
	cacheKey := fmt.Sprintf("project:%s:%s", userID, projectID)
	var resp ProjectResponse

	err := s.cache.Remember(ctx, cacheKey, 15*time.Minute, &resp, func(fetchCtx context.Context) (any, error) {
		project, err := s.repo.GetProjectByID(fetchCtx, projectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrProjectNotFound
			}
			return nil, fmt.Errorf("failed to query project: %w", err)
		}

		role := "owner"
		if project.OwnerID != userID {
			member, err := s.repo.GetProjectMember(fetchCtx, repository.GetProjectMemberParams{
				ProjectID: projectID,
				UserID:    userID,
			})
			if err != nil {
				return nil, ErrProjectNotFound // anti-IDOR: return 404 instead of 403
			}
			role = member.Role
		}

		res := s.mapProject(project)
		res.Role = role
		return res, nil
	})
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// ListProjects retrieves paginated projects accessible to the user (owned or collaborated) using keyset cursor or offset.
func (s *projectService) ListProjects(ctx context.Context, userID uuid.UUID, params PaginationParams) (*PaginatedProjectsResponse, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	if params.Cursor != "" {
		cursorTime, cursorID, err := pagination.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, domain.ValidationErrors{
				Message: "Input validation failed",
				Params: []domain.InvalidParam{
					{Name: "cursor", Reason: "invalid pagination cursor"},
				},
			}
		}

		projects, err := s.repo.ListProjectsByMemberKeyset(ctx, repository.ListProjectsByMemberKeysetParams{
			UserID:          userID,
			Limit:           limit + 1,
			CursorCreatedAt: pgtype.Timestamptz{Time: cursorTime, Valid: true},
			CursorID:        pgtype.UUID{Bytes: cursorID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query keyset projects: %w", err)
		}

		hasMore := len(projects) > int(limit)
		if hasMore {
			projects = projects[:limit]
		}

		var nextCursor string
		if len(projects) > 0 && hasMore {
			last := projects[len(projects)-1]
			nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
		}

		items := make([]ProjectResponse, len(projects))
		for i, p := range projects {
			items[i] = ProjectResponse{
				ID:          p.ID,
				OwnerID:     p.OwnerID,
				Name:        p.Name,
				Description: p.Description,
				Color:       p.Color,
				Role:        p.MemberRole,
				Version:     p.Version,
				CreatedAt:   p.CreatedAt.Time,
				UpdatedAt:   p.UpdatedAt.Time,
			}
		}

		return &PaginatedProjectsResponse{
			Items:      items,
			TotalItems: int64(len(items)),
			Limit:      limit,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	total, err := s.repo.CountProjectsByMember(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count projects: %w", err)
	}

	projects, err := s.repo.ListProjectsByMember(ctx, repository.ListProjectsByMemberParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	items := make([]ProjectResponse, len(projects))
	for i, p := range projects {
		items[i] = ProjectResponse{
			ID:          p.ID,
			OwnerID:     p.OwnerID,
			Name:        p.Name,
			Description: p.Description,
			Color:       p.Color,
			Role:        p.MemberRole,
			Version:     p.Version,
			CreatedAt:   p.CreatedAt.Time,
			UpdatedAt:   p.UpdatedAt.Time,
		}
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	hasMore := int64(offset+limit) < total
	var nextCursor string
	if len(projects) > 0 && hasMore {
		last := projects[len(projects)-1]
		nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
	}

	return &PaginatedProjectsResponse{
		Items:      items,
		TotalItems: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// UpdateProject updates project details with optimistic concurrency check (version matching). Only owner can update project metadata.
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
		return nil, ErrProjectNotFound // anti-IDOR: return 404 instead of 403
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

	// Invalidate project cache
	_ = s.cache.Delete(ctx, fmt.Sprintf("project:%s:%s", ownerID, projectID))

	res := s.mapProject(updated)
	res.Role = "owner"
	return res, nil
}

// DeleteProject soft-deletes the project owned by user. Only owner can delete project.
func (s *projectService) DeleteProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error {
	existing, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("failed to check project before deletion: %w", err)
	}

	if existing.OwnerID != ownerID {
		return ErrProjectNotFound // anti-IDOR: return 404 instead of 403
	}

	if err := s.repo.SoftDeleteProject(ctx, repository.SoftDeleteProjectParams{
		ID:      projectID,
		OwnerID: ownerID,
	}); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	// Invalidate project cache
	_ = s.cache.Delete(ctx, fmt.Sprintf("project:%s:%s", ownerID, projectID))

	return nil
}

// AddMember adds a collaborator to a project by email. Only the project owner can add members.
func (s *projectService) AddMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, input AddProjectMemberInput) (*ProjectMemberResponse, error) {
	// 1. Verify project exists and caller is owner
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to query project: %w", err)
	}
	if project.OwnerID != currentUserID {
		return nil, ErrInsufficientProjectRole
	}

	// 2. Validate role: only 'editor' or 'viewer' allowed
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role != "editor" && role != "viewer" {
		return nil, ErrInvalidProjectRole
	}

	// 3. Find target user by email
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" {
		return nil, domain.ValidationErrors{
			Message: "Input validation failed",
			Params: []domain.InvalidParam{
				{Name: "email", Reason: "email is required"},
			},
		}
	}
	targetUser, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to look up user by email: %w", err)
	}

	// Cannot add owner as collaborator
	if targetUser.ID == project.OwnerID {
		return nil, ErrProjectMemberAlreadyExists
	}

	// Check if already a member
	_, err = s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUser.ID,
	})
	if err == nil {
		return nil, ErrProjectMemberAlreadyExists
	}

	// 4. Insert project member
	pm, err := s.repo.AddProjectMember(ctx, repository.AddProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUser.ID,
		Role:      role,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add project member: %w", err)
	}

	// Invalidate target user's project cache
	_ = s.cache.Delete(ctx, fmt.Sprintf("project:%s:%s", targetUser.ID, projectID))

	return &ProjectMemberResponse{
		ID:        pm.ID,
		ProjectID: pm.ProjectID,
		UserID:    pm.UserID,
		Email:     targetUser.Email,
		FullName:  targetUser.FullName,
		Role:      pm.Role,
		CreatedAt: pm.CreatedAt.Time,
		UpdatedAt: pm.UpdatedAt.Time,
	}, nil
}

// ListMembers lists all collaborators of a project. Caller must be a project member.
func (s *projectService) ListMembers(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID) ([]ProjectMemberResponse, error) {
	// Verify caller has access to this project
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to query project: %w", err)
	}

	if project.OwnerID != currentUserID {
		_, err := s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
			ProjectID: projectID,
			UserID:    currentUserID,
		})
		if err != nil {
			return nil, ErrProjectNotFound // anti-IDOR
		}
	}

	rows, err := s.repo.ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list project members: %w", err)
	}

	members := make([]ProjectMemberResponse, len(rows))
	for i, r := range rows {
		members[i] = ProjectMemberResponse{
			ID:        r.ID,
			ProjectID: r.ProjectID,
			UserID:    r.UserID,
			Email:     r.Email,
			FullName:  r.FullName,
			Role:      r.Role,
			CreatedAt: r.CreatedAt.Time,
			UpdatedAt: r.UpdatedAt.Time,
		}
	}
	return members, nil
}

// UpdateMemberRole modifies a member's role (editor <-> viewer). Only project owner can update roles.
func (s *projectService) UpdateMemberRole(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID, input UpdateProjectMemberRoleInput) (*ProjectMemberResponse, error) {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to query project: %w", err)
	}
	if project.OwnerID != currentUserID {
		return nil, ErrInsufficientProjectRole
	}

	if targetUserID == project.OwnerID {
		return nil, ErrCannotModifyProjectOwner
	}

	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role != "editor" && role != "viewer" {
		return nil, ErrInvalidProjectRole
	}

	// Verify member exists
	_, err = s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectMemberNotFound
		}
		return nil, fmt.Errorf("failed to verify project member: %w", err)
	}

	updated, err := s.repo.UpdateProjectMemberRole(ctx, repository.UpdateProjectMemberRoleParams{
		ProjectID: projectID,
		UserID:    targetUserID,
		Role:      role,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update member role: %w", err)
	}

	// Get target user info
	targetUser, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get member user details: %w", err)
	}

	_ = s.cache.Delete(ctx, fmt.Sprintf("project:%s:%s", targetUserID, projectID))

	return &ProjectMemberResponse{
		ID:        updated.ID,
		ProjectID: updated.ProjectID,
		UserID:    updated.UserID,
		Email:     targetUser.Email,
		FullName:  targetUser.FullName,
		Role:      updated.Role,
		CreatedAt: updated.CreatedAt.Time,
		UpdatedAt: updated.UpdatedAt.Time,
	}, nil
}

// RemoveMember removes a collaborator from the project or allows a collaborator to leave.
func (s *projectService) RemoveMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID) error {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("failed to query project: %w", err)
	}

	if targetUserID == project.OwnerID {
		return ErrCannotRemoveProjectOwner
	}

	// Check permissions: Owner can remove any member; member can remove themselves (leave)
	isOwner := project.OwnerID == currentUserID
	isSelf := currentUserID == targetUserID
	if !isOwner && !isSelf {
		return ErrInsufficientProjectRole
	}

	// Verify target is actually a member
	_, err = s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectMemberNotFound
		}
		return fmt.Errorf("failed to verify member: %w", err)
	}

	if err := s.repo.DeleteProjectMember(ctx, repository.DeleteProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUserID,
	}); err != nil {
		return fmt.Errorf("failed to remove project member: %w", err)
	}

	_ = s.cache.Delete(ctx, fmt.Sprintf("project:%s:%s", targetUserID, projectID))

	return nil
}

func (s *projectService) mapProject(p repository.Project) *ProjectResponse {
	return &ProjectResponse{
		ID:          p.ID,
		OwnerID:     p.OwnerID,
		Name:        p.Name,
		Description: p.Description,
		Color:       colorOrDefault(p.Color),
		Version:     p.Version,
		CreatedAt:   p.CreatedAt.Time,
		UpdatedAt:   p.UpdatedAt.Time,
	}
}

func colorOrDefault(color string) string {
	if strings.TrimSpace(color) == "" {
		return "#3b82f6"
	}
	return color
}
