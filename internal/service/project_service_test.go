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

type mockProjectQuerier struct {
	mockQuerier
	createProjectFn        func(ctx context.Context, arg repository.CreateProjectParams) (repository.Project, error)
	getProjectByIDFn       func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	listProjectsByOwnerFn  func(ctx context.Context, arg repository.ListProjectsByOwnerParams) ([]repository.Project, error)
	listProjectsKeysetFn   func(ctx context.Context, arg repository.ListProjectsKeysetParams) ([]repository.Project, error)
	countProjectsByOwnerFn func(ctx context.Context, ownerID uuid.UUID) (int64, error)
	updateProjectFn        func(ctx context.Context, arg repository.UpdateProjectParams) (repository.Project, error)
	softDeleteProjectFn    func(ctx context.Context, arg repository.SoftDeleteProjectParams) error
}

func (m *mockProjectQuerier) ListProjectsKeyset(ctx context.Context, arg repository.ListProjectsKeysetParams) ([]repository.Project, error) {
	if m.listProjectsKeysetFn != nil {
		return m.listProjectsKeysetFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockProjectQuerier) CreateProject(ctx context.Context, arg repository.CreateProjectParams) (repository.Project, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, arg)
	}
	return repository.Project{}, nil
}

func (m *mockProjectQuerier) GetProjectByID(ctx context.Context, id uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, id)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockProjectQuerier) ListProjectsByOwner(ctx context.Context, arg repository.ListProjectsByOwnerParams) ([]repository.Project, error) {
	if m.listProjectsByOwnerFn != nil {
		return m.listProjectsByOwnerFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockProjectQuerier) CountProjectsByOwner(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	if m.countProjectsByOwnerFn != nil {
		return m.countProjectsByOwnerFn(ctx, ownerID)
	}
	return 0, nil
}

func (m *mockProjectQuerier) UpdateProject(ctx context.Context, arg repository.UpdateProjectParams) (repository.Project, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, arg)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockProjectQuerier) SoftDeleteProject(ctx context.Context, arg repository.SoftDeleteProjectParams) error {
	if m.softDeleteProjectFn != nil {
		return m.softDeleteProjectFn(ctx, arg)
	}
	return nil
}

func TestProjectService_CreateProject(t *testing.T) {
	ownerID := uuid.New()
	projectID := uuid.New()

	mock := &mockProjectQuerier{
		createProjectFn: func(_ context.Context, arg repository.CreateProjectParams) (repository.Project, error) {
			require.Equal(t, ownerID, arg.OwnerID)
			require.Equal(t, "Marketing Campaign", arg.Name)
			require.Equal(t, "#3b82f6", arg.Color) // default color
			return repository.Project{
				ID:          projectID,
				OwnerID:     arg.OwnerID,
				Name:        arg.Name,
				Description: arg.Description,
				Color:       arg.Color,
				Version:     1,
				CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UpdatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
	}

	service := NewProjectService(mock, nil)
	res, err := service.CreateProject(context.Background(), ownerID, CreateProjectInput{
		Name:        "Marketing Campaign",
		Description: "Q4 Marketing campaign",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, projectID, res.ID)
	require.Equal(t, "Marketing Campaign", res.Name)
	require.Equal(t, int32(1), res.Version)
}

func TestProjectService_GetProjectByID(t *testing.T) {
	ownerID := uuid.New()
	projectID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:        id,
					OwnerID:   ownerID,
					Name:      "Alpha",
					Version:   1,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), ownerID, projectID)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, "Alpha", res.Name)
	})

	t.Run("NotFound", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return repository.Project{}, pgx.ErrNoRows
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), ownerID, projectID)
		require.ErrorIs(t, err, ErrProjectNotFound)
		require.Nil(t, res)
	})

	t.Run("AntiIDOR_NotOwner", func(t *testing.T) {
		differentOwner := uuid.New()
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: differentOwner,
					Name:    "Secret Project",
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), ownerID, projectID)
		require.ErrorIs(t, err, ErrProjectNotFound)
		require.Nil(t, res)
	})
}

func TestProjectService_ListProjects(t *testing.T) {
	ownerID := uuid.New()

	mock := &mockProjectQuerier{
		countProjectsByOwnerFn: func(_ context.Context, id uuid.UUID) (int64, error) {
			require.Equal(t, ownerID, id)
			return 25, nil
		},
		listProjectsByOwnerFn: func(_ context.Context, arg repository.ListProjectsByOwnerParams) ([]repository.Project, error) {
			require.Equal(t, int32(10), arg.Limit)
			require.Equal(t, int32(0), arg.Offset)
			return []repository.Project{
				{ID: uuid.New(), OwnerID: ownerID, Name: "P1", Version: 1},
				{ID: uuid.New(), OwnerID: ownerID, Name: "P2", Version: 1},
			}, nil
		},
	}

	service := NewProjectService(mock, nil)
	res, err := service.ListProjects(context.Background(), ownerID, PaginationParams{
		Page:  1,
		Limit: 10,
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, int64(25), res.TotalItems)
	require.Equal(t, int32(3), res.TotalPages) // ceil(25 / 10) = 3
	require.Len(t, res.Items, 2)

	// Keyset pagination
	mockKeyset := &mockProjectQuerier{
		listProjectsKeysetFn: func(_ context.Context, _ repository.ListProjectsKeysetParams) ([]repository.Project, error) {
			return []repository.Project{
				{ID: uuid.New(), OwnerID: ownerID, Name: "P1", Version: 1, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
			}, nil
		},
	}
	svcKeyset := NewProjectService(mockKeyset, nil)
	resKeyset, err := svcKeyset.ListProjects(context.Background(), ownerID, PaginationParams{
		Cursor: "MjAyNi0xMS0wNlQxNjo1Mzo1My40NTAwMjVafDAxYTFiMWM3LWYyYWEtNzk5Zi1hY2FmLWMwNDM5OTZlOGE4OQ",
		Limit:  10,
	})
	require.NoError(t, err)
	require.NotNil(t, resKeyset)
	require.Len(t, resKeyset.Items, 1)
}

func TestProjectService_UpdateProject(t *testing.T) {
	ownerID := uuid.New()
	projectID := uuid.New()

	t.Run("Success_VersionMatches", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: ownerID,
					Name:    "Old Name",
					Version: 1,
				}, nil
			},
			updateProjectFn: func(_ context.Context, arg repository.UpdateProjectParams) (repository.Project, error) {
				require.Equal(t, int32(1), arg.Version)
				return repository.Project{
					ID:        arg.ID,
					OwnerID:   arg.OwnerID,
					Name:      arg.Name,
					Version:   2,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.UpdateProject(context.Background(), ownerID, projectID, UpdateProjectInput{
			Name:    "New Name",
			Version: 1,
		})

		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, "New Name", res.Name)
		require.Equal(t, int32(2), res.Version)
	})

	t.Run("Conflict_VersionMismatch", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: ownerID,
					Name:    "Updated Name",
					Version: 3, // Current DB version is 3
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		// Client sends stale version 1
		res, err := service.UpdateProject(context.Background(), ownerID, projectID, UpdateProjectInput{
			Name:    "Stale Update",
			Version: 1,
		})

		require.ErrorIs(t, err, ErrOptimisticLockConflict)
		require.Nil(t, res)
	})
}

func TestProjectService_DeleteProject(t *testing.T) {
	ownerID := uuid.New()
	projectID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		deleted := false
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: ownerID,
				}, nil
			},
			softDeleteProjectFn: func(_ context.Context, arg repository.SoftDeleteProjectParams) error {
				if arg.ID == projectID && arg.OwnerID == ownerID {
					deleted = true
				}
				return nil
			},
		}

		service := NewProjectService(mock, nil)
		err := service.DeleteProject(context.Background(), ownerID, projectID)

		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("AntiIDOR_NotOwner", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: uuid.New(), // different owner
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		err := service.DeleteProject(context.Background(), ownerID, projectID)
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}
