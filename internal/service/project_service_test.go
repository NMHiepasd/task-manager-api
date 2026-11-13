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
	createProjectFn              func(ctx context.Context, arg repository.CreateProjectParams) (repository.Project, error)
	getProjectByIDFn             func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	listProjectsByOwnerFn        func(ctx context.Context, arg repository.ListProjectsByOwnerParams) ([]repository.Project, error)
	listProjectsKeysetFn         func(ctx context.Context, arg repository.ListProjectsKeysetParams) ([]repository.Project, error)
	listProjectsByMemberFn       func(ctx context.Context, arg repository.ListProjectsByMemberParams) ([]repository.ListProjectsByMemberRow, error)
	listProjectsByMemberKeysetFn func(ctx context.Context, arg repository.ListProjectsByMemberKeysetParams) ([]repository.ListProjectsByMemberKeysetRow, error)
	countProjectsByMemberFn      func(ctx context.Context, userID uuid.UUID) (int64, error)
	countProjectsByOwnerFn       func(ctx context.Context, ownerID uuid.UUID) (int64, error)
	updateProjectFn              func(ctx context.Context, arg repository.UpdateProjectParams) (repository.Project, error)
	softDeleteProjectFn          func(ctx context.Context, arg repository.SoftDeleteProjectParams) error
	addProjectMemberFn           func(ctx context.Context, arg repository.AddProjectMemberParams) (repository.ProjectMember, error)
	getProjectMemberFn           func(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error)
	listProjectMembersFn         func(ctx context.Context, projectID uuid.UUID) ([]repository.ListProjectMembersRow, error)
	updateProjectMemberRoleFn    func(ctx context.Context, arg repository.UpdateProjectMemberRoleParams) (repository.ProjectMember, error)
	deleteProjectMemberFn        func(ctx context.Context, arg repository.DeleteProjectMemberParams) error
	getUserByEmailFn             func(ctx context.Context, email string) (repository.User, error)
	getUserByIDFn                func(ctx context.Context, id uuid.UUID) (repository.User, error)
}

func (m *mockProjectQuerier) ListProjectsKeyset(ctx context.Context, arg repository.ListProjectsKeysetParams) ([]repository.Project, error) {
	if m.listProjectsKeysetFn != nil {
		return m.listProjectsKeysetFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockProjectQuerier) ListProjectsByMember(ctx context.Context, arg repository.ListProjectsByMemberParams) ([]repository.ListProjectsByMemberRow, error) {
	if m.listProjectsByMemberFn != nil {
		return m.listProjectsByMemberFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockProjectQuerier) ListProjectsByMemberKeyset(ctx context.Context, arg repository.ListProjectsByMemberKeysetParams) ([]repository.ListProjectsByMemberKeysetRow, error) {
	if m.listProjectsByMemberKeysetFn != nil {
		return m.listProjectsByMemberKeysetFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockProjectQuerier) CountProjectsByMember(ctx context.Context, userID uuid.UUID) (int64, error) {
	if m.countProjectsByMemberFn != nil {
		return m.countProjectsByMemberFn(ctx, userID)
	}
	return 0, nil
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

func (m *mockProjectQuerier) AddProjectMember(ctx context.Context, arg repository.AddProjectMemberParams) (repository.ProjectMember, error) {
	if m.addProjectMemberFn != nil {
		return m.addProjectMemberFn(ctx, arg)
	}
	return repository.ProjectMember{}, nil
}

func (m *mockProjectQuerier) GetProjectMember(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	if m.getProjectMemberFn != nil {
		return m.getProjectMemberFn(ctx, arg)
	}
	return repository.ProjectMember{}, pgx.ErrNoRows
}

func (m *mockProjectQuerier) ListProjectMembers(ctx context.Context, projectID uuid.UUID) ([]repository.ListProjectMembersRow, error) {
	if m.listProjectMembersFn != nil {
		return m.listProjectMembersFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockProjectQuerier) UpdateProjectMemberRole(ctx context.Context, arg repository.UpdateProjectMemberRoleParams) (repository.ProjectMember, error) {
	if m.updateProjectMemberRoleFn != nil {
		return m.updateProjectMemberRoleFn(ctx, arg)
	}
	return repository.ProjectMember{}, nil
}

func (m *mockProjectQuerier) DeleteProjectMember(ctx context.Context, arg repository.DeleteProjectMemberParams) error {
	if m.deleteProjectMemberFn != nil {
		return m.deleteProjectMemberFn(ctx, arg)
	}
	return nil
}

func (m *mockProjectQuerier) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, email)
	}
	return repository.User{}, pgx.ErrNoRows
}

func (m *mockProjectQuerier) GetUserByID(ctx context.Context, id uuid.UUID) (repository.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}
	return repository.User{}, pgx.ErrNoRows
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
	require.Equal(t, "owner", res.Role)
	require.Equal(t, "Marketing Campaign", res.Name)
}

func TestProjectService_GetProjectByID(t *testing.T) {
	ownerID := uuid.New()
	memberID := uuid.New()
	projectID := uuid.New()

	t.Run("Success_Owner", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				require.Equal(t, projectID, id)
				return repository.Project{
					ID:        projectID,
					OwnerID:   ownerID,
					Name:      "Q4 Roadmap",
					Version:   1,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), ownerID, projectID)

		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, "Q4 Roadmap", res.Name)
		require.Equal(t, "owner", res.Role)
	})

	t.Run("Success_Collaborator", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:        projectID,
					OwnerID:   ownerID,
					Name:      "Shared Project",
					Version:   1,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
			getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				require.Equal(t, projectID, arg.ProjectID)
				require.Equal(t, memberID, arg.UserID)
				return repository.ProjectMember{
					ProjectID: projectID,
					UserID:    memberID,
					Role:      "editor",
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), memberID, projectID)

		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, "Shared Project", res.Name)
		require.Equal(t, "editor", res.Role)
	})

	t.Run("AntiIDOR_NotOwnerOrMember", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{
					ID:      id,
					OwnerID: ownerID,
				}, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{}, pgx.ErrNoRows
			},
		}

		service := NewProjectService(mock, nil)
		res, err := service.GetProjectByID(context.Background(), uuid.New(), projectID)
		require.ErrorIs(t, err, ErrProjectNotFound)
		require.Nil(t, res)
	})
}

func TestProjectService_ListProjects(t *testing.T) {
	ownerID := uuid.New()

	mock := &mockProjectQuerier{
		countProjectsByMemberFn: func(_ context.Context, id uuid.UUID) (int64, error) {
			require.Equal(t, ownerID, id)
			return 25, nil
		},
		listProjectsByMemberFn: func(_ context.Context, arg repository.ListProjectsByMemberParams) ([]repository.ListProjectsByMemberRow, error) {
			require.Equal(t, int32(10), arg.Limit)
			require.Equal(t, int32(0), arg.Offset)
			return []repository.ListProjectsByMemberRow{
				{ID: uuid.New(), OwnerID: ownerID, Name: "P1", MemberRole: "owner", Version: 1},
				{ID: uuid.New(), OwnerID: ownerID, Name: "P2", MemberRole: "editor", Version: 1},
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
	require.Equal(t, "owner", res.Items[0].Role)
	require.Equal(t, "editor", res.Items[1].Role)

	// Keyset pagination
	mockKeyset := &mockProjectQuerier{
		listProjectsByMemberKeysetFn: func(_ context.Context, _ repository.ListProjectsByMemberKeysetParams) ([]repository.ListProjectsByMemberKeysetRow, error) {
			return []repository.ListProjectsByMemberKeysetRow{
				{ID: uuid.New(), OwnerID: ownerID, Name: "P1", MemberRole: "owner", Version: 1, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
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
					Version: 3,
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
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
					OwnerID: uuid.New(),
				}, nil
			},
		}

		service := NewProjectService(mock, nil)
		err := service.DeleteProject(context.Background(), ownerID, projectID)
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}

func TestProjectService_MemberCollaboration(t *testing.T) {
	ownerID := uuid.New()
	targetUserID := uuid.New()
	projectID := uuid.New()

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID,
		Name:    "Collab Project",
	}

	targetUser := repository.User{
		ID:       targetUserID,
		Email:    "collab@example.com",
		FullName: "Collab User",
	}

	t.Run("AddMember_Success", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getUserByEmailFn: func(_ context.Context, email string) (repository.User, error) {
				require.Equal(t, "collab@example.com", email)
				return targetUser, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{}, pgx.ErrNoRows // not yet member
			},
			addProjectMemberFn: func(_ context.Context, arg repository.AddProjectMemberParams) (repository.ProjectMember, error) {
				require.Equal(t, projectID, arg.ProjectID)
				require.Equal(t, targetUserID, arg.UserID)
				require.Equal(t, "editor", arg.Role)
				return repository.ProjectMember{
					ID:        uuid.New(),
					ProjectID: arg.ProjectID,
					UserID:    arg.UserID,
					Role:      arg.Role,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewProjectService(mock, nil)
		resp, err := svc.AddMember(context.Background(), ownerID, projectID, AddProjectMemberInput{
			Email: "collab@example.com",
			Role:  "editor",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "editor", resp.Role)
		require.Equal(t, "collab@example.com", resp.Email)
	})

	t.Run("AddMember_ForbiddenIfNotOwner", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}

		svc := NewProjectService(mock, nil)
		_, err := svc.AddMember(context.Background(), uuid.New(), projectID, AddProjectMemberInput{
			Email: "collab@example.com",
			Role:  "editor",
		})
		require.ErrorIs(t, err, ErrInsufficientProjectRole)
	})

	t.Run("AddMember_InvalidRole", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}

		svc := NewProjectService(mock, nil)
		_, err := svc.AddMember(context.Background(), ownerID, projectID, AddProjectMemberInput{
			Email: "collab@example.com",
			Role:  "admin", // invalid
		})
		require.ErrorIs(t, err, ErrInvalidProjectRole)
	})

	t.Run("AddMember_AlreadyMember", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getUserByEmailFn: func(_ context.Context, _ string) (repository.User, error) {
				return targetUser, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{Role: "viewer"}, nil // already member
			},
		}

		svc := NewProjectService(mock, nil)
		_, err := svc.AddMember(context.Background(), ownerID, projectID, AddProjectMemberInput{
			Email: "collab@example.com",
			Role:  "editor",
		})
		require.ErrorIs(t, err, ErrProjectMemberAlreadyExists)
	})

	t.Run("ListMembers_Success", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			listProjectMembersFn: func(_ context.Context, id uuid.UUID) ([]repository.ListProjectMembersRow, error) {
				return []repository.ListProjectMembersRow{
					{ID: uuid.New(), ProjectID: id, UserID: ownerID, Role: "owner", Email: "owner@example.com", FullName: "Owner"},
					{ID: uuid.New(), ProjectID: id, UserID: targetUserID, Role: "editor", Email: "collab@example.com", FullName: "Collab"},
				}, nil
			},
		}

		svc := NewProjectService(mock, nil)
		members, err := svc.ListMembers(context.Background(), ownerID, projectID)
		require.NoError(t, err)
		require.Len(t, members, 2)
		require.Equal(t, "owner", members[0].Role)
		require.Equal(t, "editor", members[1].Role)
	})

	t.Run("UpdateMemberRole_Success", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{ProjectID: projectID, UserID: targetUserID, Role: "editor"}, nil
			},
			updateProjectMemberRoleFn: func(_ context.Context, arg repository.UpdateProjectMemberRoleParams) (repository.ProjectMember, error) {
				require.Equal(t, "viewer", arg.Role)
				return repository.ProjectMember{
					ID:        uuid.New(),
					ProjectID: arg.ProjectID,
					UserID:    arg.UserID,
					Role:      arg.Role,
				}, nil
			},
			getUserByIDFn: func(_ context.Context, _ uuid.UUID) (repository.User, error) {
				return targetUser, nil
			},
		}

		svc := NewProjectService(mock, nil)
		updated, err := svc.UpdateMemberRole(context.Background(), ownerID, projectID, targetUserID, UpdateProjectMemberRoleInput{Role: "viewer"})
		require.NoError(t, err)
		require.Equal(t, "viewer", updated.Role)
	})

	t.Run("UpdateMemberRole_CannotModifyOwner", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}

		svc := NewProjectService(mock, nil)
		_, err := svc.UpdateMemberRole(context.Background(), ownerID, projectID, ownerID, UpdateProjectMemberRoleInput{Role: "viewer"})
		require.ErrorIs(t, err, ErrCannotModifyProjectOwner)
	})

	t.Run("RemoveMember_OwnerRemovesMember_Success", func(t *testing.T) {
		deleted := false
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{ProjectID: projectID, UserID: targetUserID}, nil
			},
			deleteProjectMemberFn: func(_ context.Context, _ repository.DeleteProjectMemberParams) error {
				deleted = true
				return nil
			},
		}

		svc := NewProjectService(mock, nil)
		err := svc.RemoveMember(context.Background(), ownerID, projectID, targetUserID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("RemoveMember_MemberLeaves_Success", func(t *testing.T) {
		deleted := false
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{ProjectID: projectID, UserID: targetUserID}, nil
			},
			deleteProjectMemberFn: func(_ context.Context, _ repository.DeleteProjectMemberParams) error {
				deleted = true
				return nil
			},
		}

		svc := NewProjectService(mock, nil)
		// targetUserID removing itself (leaving)
		err := svc.RemoveMember(context.Background(), targetUserID, projectID, targetUserID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("RemoveMember_CannotRemoveOwner", func(t *testing.T) {
		mock := &mockProjectQuerier{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}

		svc := NewProjectService(mock, nil)
		err := svc.RemoveMember(context.Background(), ownerID, projectID, ownerID)
		require.ErrorIs(t, err, ErrCannotRemoveProjectOwner)
	})
}
