package service_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/service"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_TagCacheAside_And_Invalidation(t *testing.T) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/2", "", 2)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer c.Close()

	userID := uuid.New()
	var listCalls int32

	mock := &mockQuerierWithHooks{
		listTagsByUserFn: func(_ context.Context, uID uuid.UUID) ([]repository.Tag, error) {
			atomic.AddInt32(&listCalls, 1)
			return []repository.Tag{
				{
					ID:        uuid.New(),
					UserID:    uID,
					Name:      "Cached Tag",
					Color:     "#3b82f6",
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			}, nil
		},
		createTagFn: func(_ context.Context, arg repository.CreateTagParams) (repository.Tag, error) {
			return repository.Tag{
				ID:        uuid.New(),
				UserID:    arg.UserID,
				Name:      arg.Name,
				Color:     arg.Color,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
	}

	tagSvc := service.NewTagService(mock, c)

	// Clean any pre-existing cache
	_ = c.Delete(ctx, fmt.Sprintf("user:%s:tags", userID))

	// 1. First read: cache miss -> calls DB repository
	tags1, err := tagSvc.ListTags(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, tags1, 1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&listCalls))

	// 2. Second read: cache hit -> DB repository NOT called again
	tags2, err := tagSvc.ListTags(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, tags2, 1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&listCalls), "Must read from Redis cache without hitting DB")

	// 3. Mutation: CreateTag must invalidate the cache key
	_, err = tagSvc.CreateTag(ctx, userID, service.CreateTagInput{Name: "New Tag"})
	require.NoError(t, err)

	// 4. Third read: cache was invalidated -> calls DB repository again
	tags3, err := tagSvc.ListTags(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, tags3, 1)
	assert.Equal(t, int32(2), atomic.LoadInt32(&listCalls), "Must query DB after cache invalidation")
}

func TestService_ProjectCacheAside_And_Invalidation(t *testing.T) {
	ctx := context.Background()
	c, err := cache.NewRedisCache(ctx, "redis://localhost:6379/2", "", 2)
	if err != nil {
		t.Skip("Skipping Redis test: local redis container not available on localhost:6379")
		return
	}
	defer c.Close()

	userID := uuid.New()
	projectID := uuid.New()
	var getCalls int32

	mock := &mockQuerierWithHooks{
		getProjectByIDFn: func(_ context.Context, pID uuid.UUID) (repository.Project, error) {
			atomic.AddInt32(&getCalls, 1)
			return repository.Project{
				ID:          pID,
				OwnerID:     userID,
				Name:        "Cached Project",
				Description: "Fast Redis Reads",
				Color:       "#10b981",
				Version:     1,
				CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UpdatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		updateProjectFn: func(_ context.Context, arg repository.UpdateProjectParams) (repository.Project, error) {
			return repository.Project{
				ID:          arg.ID,
				OwnerID:     arg.OwnerID,
				Name:        arg.Name,
				Description: arg.Description,
				Color:       arg.Color,
				Version:     arg.Version + 1,
				CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UpdatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
	}

	projectSvc := service.NewProjectService(mock, c)

	// Clean pre-existing cache
	_ = c.Delete(ctx, fmt.Sprintf("project:%s:%s", userID, projectID))

	// 1. First read: DB called
	p1, err := projectSvc.GetProjectByID(ctx, userID, projectID)
	require.NoError(t, err)
	assert.Equal(t, "Cached Project", p1.Name)
	assert.Equal(t, int32(1), atomic.LoadInt32(&getCalls))

	// 2. Second read: Cache hit
	p2, err := projectSvc.GetProjectByID(ctx, userID, projectID)
	require.NoError(t, err)
	assert.Equal(t, "Cached Project", p2.Name)
	assert.Equal(t, int32(1), atomic.LoadInt32(&getCalls), "Must read from Redis cache")

	// 3. Update project: Verifies existing project in DB (call #2) and invalidates cache
	_, err = projectSvc.UpdateProject(ctx, userID, projectID, service.UpdateProjectInput{Name: "Updated Project", Version: 1})
	require.NoError(t, err)

	// 4. Third read: DB called again (call #3) because cache was invalidated
	_, err = projectSvc.GetProjectByID(ctx, userID, projectID)
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&getCalls), "Must query DB after invalidation")
}

// mockQuerierWithHooks allows selective function overrides while stubbing the rest.
type mockQuerierWithHooks struct {
	listTagsByUserFn func(ctx context.Context, userID uuid.UUID) ([]repository.Tag, error)
	createTagFn      func(ctx context.Context, arg repository.CreateTagParams) (repository.Tag, error)
	getProjectByIDFn func(ctx context.Context, projectID uuid.UUID) (repository.Project, error)
	updateProjectFn  func(ctx context.Context, arg repository.UpdateProjectParams) (repository.Project, error)
}

func (m *mockQuerierWithHooks) ListTagsByUser(ctx context.Context, userID uuid.UUID) ([]repository.Tag, error) {
	if m.listTagsByUserFn != nil {
		return m.listTagsByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockQuerierWithHooks) CreateTag(ctx context.Context, arg repository.CreateTagParams) (repository.Tag, error) {
	if m.createTagFn != nil {
		return m.createTagFn(ctx, arg)
	}
	return repository.Tag{}, nil
}

func (m *mockQuerierWithHooks) GetProjectByID(ctx context.Context, projectID uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, projectID)
	}
	return repository.Project{}, nil
}

func (m *mockQuerierWithHooks) UpdateProject(ctx context.Context, arg repository.UpdateProjectParams) (repository.Project, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, arg)
	}
	return repository.Project{}, nil
}

// Stubs for remainder of repository.Querier
func (m *mockQuerierWithHooks) CreateIdempotencyKey(context.Context, repository.CreateIdempotencyKeyParams) error {
	return nil
}

func (m *mockQuerierWithHooks) DeleteOldIdempotencyKeys(context.Context, pgtype.Timestamptz) error {
	return nil
}

func (m *mockQuerierWithHooks) GetIdempotencyKey(context.Context, repository.GetIdempotencyKeyParams) (repository.IdempotencyKey, error) {
	return repository.IdempotencyKey{}, nil
}

func (m *mockQuerierWithHooks) UpdateIdempotencyKeyResponse(context.Context, repository.UpdateIdempotencyKeyResponseParams) error {
	return nil
}

func (m *mockQuerierWithHooks) ListTasksDueSoon(context.Context, repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) CreateUser(context.Context, repository.CreateUserParams) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockQuerierWithHooks) GetUserByEmail(context.Context, string) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockQuerierWithHooks) GetUserByID(context.Context, uuid.UUID) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockQuerierWithHooks) CreateRefreshToken(context.Context, repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
	return repository.RefreshToken{}, nil
}

func (m *mockQuerierWithHooks) GetRefreshTokenByHash(context.Context, string) (repository.RefreshToken, error) {
	return repository.RefreshToken{}, nil
}
func (m *mockQuerierWithHooks) RevokeRefreshToken(context.Context, string) error { return nil }
func (m *mockQuerierWithHooks) RevokeAllUserRefreshTokens(context.Context, uuid.UUID) error {
	return nil
}
func (m *mockQuerierWithHooks) DeleteExpiredRefreshTokens(context.Context) error { return nil }
func (m *mockQuerierWithHooks) CreateProject(context.Context, repository.CreateProjectParams) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockQuerierWithHooks) ListProjectsByOwner(context.Context, repository.ListProjectsByOwnerParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListProjectsKeyset(context.Context, repository.ListProjectsKeysetParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) CountProjectsByOwner(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockQuerierWithHooks) SoftDeleteProject(context.Context, repository.SoftDeleteProjectParams) error {
	return nil
}

func (m *mockQuerierWithHooks) CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockQuerierWithHooks) GetTaskByID(context.Context, uuid.UUID) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockQuerierWithHooks) ListTasks(context.Context, repository.ListTasksParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListTasksKeyset(context.Context, repository.ListTasksKeysetParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) CountTasks(context.Context, repository.CountTasksParams) (int64, error) {
	return 0, nil
}

func (m *mockQuerierWithHooks) UpdateTask(context.Context, repository.UpdateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}
func (m *mockQuerierWithHooks) SoftDeleteTask(context.Context, uuid.UUID) error { return nil }
func (m *mockQuerierWithHooks) GetTagByID(context.Context, uuid.UUID) (repository.Tag, error) {
	return repository.Tag{}, nil
}

func (m *mockQuerierWithHooks) DeleteTag(context.Context, repository.DeleteTagParams) error {
	return nil
}

func (m *mockQuerierWithHooks) AddTagToTask(context.Context, repository.AddTagToTaskParams) error {
	return nil
}

func (m *mockQuerierWithHooks) RemoveTagFromTask(context.Context, repository.RemoveTagFromTaskParams) error {
	return nil
}
func (m *mockQuerierWithHooks) ClearTaskTags(context.Context, uuid.UUID) error { return nil }
func (m *mockQuerierWithHooks) GetTagsForTask(context.Context, uuid.UUID) ([]repository.Tag, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) GetTagsForTasks(context.Context, []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListTasksByCreator(context.Context, repository.ListTasksByCreatorParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListUsers(context.Context, repository.ListUsersParams) ([]repository.User, error) {
	return nil, nil
}
func (m *mockQuerierWithHooks) SoftDeleteUser(context.Context, uuid.UUID) error { return nil }
func (m *mockQuerierWithHooks) UpdateUser(context.Context, repository.UpdateUserParams) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockQuerierWithHooks) AddProjectMember(context.Context, repository.AddProjectMemberParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}

func (m *mockQuerierWithHooks) CountProjectMembers(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockQuerierWithHooks) CountProjectsByMember(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockQuerierWithHooks) DeleteProjectMember(context.Context, repository.DeleteProjectMemberParams) error {
	return nil
}

func (m *mockQuerierWithHooks) GetProjectMember(context.Context, repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}

func (m *mockQuerierWithHooks) GetProjectMemberWithUser(context.Context, repository.GetProjectMemberWithUserParams) (repository.GetProjectMemberWithUserRow, error) {
	return repository.GetProjectMemberWithUserRow{}, nil
}

func (m *mockQuerierWithHooks) ListProjectMembers(context.Context, uuid.UUID) ([]repository.ListProjectMembersRow, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListProjectsByMember(context.Context, repository.ListProjectsByMemberParams) ([]repository.ListProjectsByMemberRow, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) ListProjectsByMemberKeyset(context.Context, repository.ListProjectsByMemberKeysetParams) ([]repository.ListProjectsByMemberKeysetRow, error) {
	return nil, nil
}

func (m *mockQuerierWithHooks) UpdateProjectMemberRole(context.Context, repository.UpdateProjectMemberRoleParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}
