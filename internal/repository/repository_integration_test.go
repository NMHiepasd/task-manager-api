//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"task-manager-api/internal/repository"
)

var (
	testStore repository.Store
	testPool  *pgxpool.Pool
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_task_manager"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start testcontainer: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse pool config: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	testPool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect pool: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Apply schema migrations
	migrationFiles := []string{
		filepath.Join("..", "..", "migrations", "000001_create_initial_schema.up.sql"),
		filepath.Join("..", "..", "migrations", "000002_add_fts_and_indexes.up.sql"),
	}

	for _, file := range migrationFiles {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read migration file %s: %v\n", file, err)
			testPool.Close()
			_ = pgContainer.Terminate(ctx)
			os.Exit(1)
		}

		if _, err := testPool.Exec(ctx, string(sqlBytes)); err != nil {
			fmt.Fprintf(os.Stderr, "failed to execute migration %s: %v\n", file, err)
			testPool.Close()
			_ = pgContainer.Terminate(ctx)
			os.Exit(1)
		}
	}

	testStore = repository.NewStore(testPool)

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestIntegration_UserAndProjectCRUD(t *testing.T) {
	ctx := context.Background()

	// 1. Create User
	email := fmt.Sprintf("user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "Integration User",
		Role:         "user",
	})
	require.NoError(t, err)
	require.NotEmpty(t, user.ID)
	assert.Equal(t, email, user.Email)

	// Verify GetUserByEmail
	fetchedUser, err := testStore.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, user.ID, fetchedUser.ID)

	// 2. Create Project
	proj, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     user.ID,
		Name:        "Backend Architecture",
		Description: "Design scalable REST services",
		Color:       "#3b82f6",
	})
	require.NoError(t, err)
	assert.Equal(t, user.ID, proj.OwnerID)
	assert.Equal(t, int32(1), proj.Version)

	// 3. Count & List Projects
	count, err := testStore.CountProjectsByOwner(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	projects, err := testStore.ListProjectsByOwner(ctx, repository.ListProjectsByOwnerParams{
		OwnerID: user.ID,
		Limit:   10,
		Offset:  0,
	})
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, proj.ID, projects[0].ID)

	// 4. Update Project with Optimistic Locking
	updatedProj, err := testStore.UpdateProject(ctx, repository.UpdateProjectParams{
		ID:          proj.ID,
		OwnerID:     user.ID,
		Name:        "Backend Architecture v2",
		Description: "Updated description",
		Color:       "#10b981",
		Version:     proj.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), updatedProj.Version)
	assert.Equal(t, "Backend Architecture v2", updatedProj.Name)

	// Stale update with old version should fail with ErrNoRows (version mismatch)
	_, err = testStore.UpdateProject(ctx, repository.UpdateProjectParams{
		ID:          proj.ID,
		OwnerID:     user.ID,
		Name:        "Stale Write",
		Description: "Should conflict",
		Color:       "#ef4444",
		Version:     1, // current version is 2
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	// 5. Soft Delete
	err = testStore.SoftDeleteProject(ctx, repository.SoftDeleteProjectParams{
		ID:      proj.ID,
		OwnerID: user.ID,
	})
	require.NoError(t, err)

	// After soft delete, Count should be 0
	countAfterDelete, err := testStore.CountProjectsByOwner(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), countAfterDelete)
}

func TestIntegration_TasksKeysetPagination(t *testing.T) {
	ctx := context.Background()

	// Setup user & project
	email := fmt.Sprintf("keyset_user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "Keyset Tester",
		Role:         "user",
	})
	require.NoError(t, err)

	proj, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     user.ID,
		Name:        "Keyset Project",
		Description: "Testing pagination",
		Color:       "#6366f1",
	})
	require.NoError(t, err)

	pID := pgtype.UUID{Bytes: proj.ID, Valid: true}

	// Insert 4 tasks with slight delay to ensure strictly distinct timestamps
	createdTasks := make([]repository.Task, 4)
	for i := 0; i < 4; i++ {
		task, err := testStore.CreateTask(ctx, repository.CreateTaskParams{
			ProjectID:   pID,
			CreatorID:   user.ID,
			Title:       fmt.Sprintf("Keyset Task %d", i+1),
			Description: fmt.Sprintf("Description for task %d", i+1),
			Status:      "todo",
			Priority:    "medium",
		})
		require.NoError(t, err)
		createdTasks[i] = task
		time.Sleep(10 * time.Millisecond)
	}

	// Page 1: Query first 2 tasks (ordered created_at DESC, id DESC)
	page1, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID: user.ID,
		Limit:     2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, createdTasks[3].ID, page1[0].ID) // newest first
	assert.Equal(t, createdTasks[2].ID, page1[1].ID)

	// Page 2: Keyset seek using cursor from last item of page 1
	cursorTask := page1[1]
	page2, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID:       user.ID,
		Limit:           2,
		CursorCreatedAt: cursorTask.CreatedAt,
		CursorID:        pgtype.UUID{Bytes: cursorTask.ID, Valid: true},
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, createdTasks[1].ID, page2[0].ID)
	assert.Equal(t, createdTasks[0].ID, page2[1].ID)

	// Page 3: Keyset seek beyond the end
	cursorTaskPage2 := page2[1]
	page3, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID:       user.ID,
		Limit:           2,
		CursorCreatedAt: cursorTaskPage2.CreatedAt,
		CursorID:        pgtype.UUID{Bytes: cursorTaskPage2.ID, Valid: true},
	})
	require.NoError(t, err)
	assert.Empty(t, page3)
}

func TestIntegration_FullTextSearch_GIN(t *testing.T) {
	ctx := context.Background()

	email := fmt.Sprintf("fts_user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "FTS Tester",
		Role:         "user",
	})
	require.NoError(t, err)

	// Create 3 distinct tasks
	_, err = testStore.CreateTask(ctx, repository.CreateTaskParams{
		CreatorID:   user.ID,
		Title:       "Setup Kubernetes cluster",
		Description: "Configure ingress controller and cert-manager",
		Status:      "todo",
		Priority:    "high",
	})
	require.NoError(t, err)

	_, err = testStore.CreateTask(ctx, repository.CreateTaskParams{
		CreatorID:   user.ID,
		Title:       "PostgreSQL Index Optimization",
		Description: "Add GIN expression index for sub-millisecond search",
		Status:      "in_progress",
		Priority:    "medium",
	})
	require.NoError(t, err)

	_, err = testStore.CreateTask(ctx, repository.CreateTaskParams{
		CreatorID:   user.ID,
		Title:       "Docker multi-stage container build",
		Description: "Produce minimal alpine production images",
		Status:      "completed",
		Priority:    "low",
	})
	require.NoError(t, err)

	// Test 1: Search "Kubernetes"
	resK8s, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID: user.ID,
		Limit:     10,
		Search:    pgtype.Text{String: "Kubernetes", Valid: true},
	})
	require.NoError(t, err)
	require.Len(t, resK8s, 1)
	assert.Equal(t, "Setup Kubernetes cluster", resK8s[0].Title)

	// Test 2: Search description term "sub-millisecond"
	resFTS, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID: user.ID,
		Limit:     10,
		Search:    pgtype.Text{String: "millisecond", Valid: true},
	})
	require.NoError(t, err)
	require.Len(t, resFTS, 1)
	assert.Equal(t, "PostgreSQL Index Optimization", resFTS[0].Title)

	// Test 3: Search stemming "building" matches "build"
	resStem, err := testStore.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
		CreatorID: user.ID,
		Limit:     10,
		Search:    pgtype.Text{String: "building", Valid: true},
	})
	require.NoError(t, err)
	require.Len(t, resStem, 1)
	assert.Equal(t, "Docker multi-stage container build", resStem[0].Title)
}

func TestIntegration_TransactionWithTx_RollbackAndCommit(t *testing.T) {
	ctx := context.Background()

	email := fmt.Sprintf("tx_user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "Tx Tester",
		Role:         "user",
	})
	require.NoError(t, err)

	task, err := testStore.CreateTask(ctx, repository.CreateTaskParams{
		CreatorID:   user.ID,
		Title:       "Task with batch tags",
		Description: "Testing WithTx atomicity",
		Status:      "todo",
		Priority:    "high",
	})
	require.NoError(t, err)

	tag1, err := testStore.CreateTag(ctx, repository.CreateTagParams{
		UserID: user.ID,
		Name:   "backend",
		Color:  "#3b82f6",
	})
	require.NoError(t, err)

	tag2, err := testStore.CreateTag(ctx, repository.CreateTagParams{
		UserID: user.ID,
		Name:   "critical",
		Color:  "#ef4444",
	})
	require.NoError(t, err)

	// Case 1: Successful Transaction Commit
	err = testStore.WithTx(ctx, func(q repository.Querier) error {
		if err := q.ClearTaskTags(ctx, task.ID); err != nil {
			return err
		}
		if err := q.AddTagToTask(ctx, repository.AddTagToTaskParams{TaskID: task.ID, TagID: tag1.ID}); err != nil {
			return err
		}
		if err := q.AddTagToTask(ctx, repository.AddTagToTaskParams{TaskID: task.ID, TagID: tag2.ID}); err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)

	tags, err := testStore.GetTagsForTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Len(t, tags, 2)

	// Case 2: Transaction Rollback on Error
	tag3, err := testStore.CreateTag(ctx, repository.CreateTagParams{
		UserID: user.ID,
		Name:   "should-rollback",
		Color:  "#10b981",
	})
	require.NoError(t, err)

	forcedErr := errors.New("simulated failure after adding tag")
	err = testStore.WithTx(ctx, func(q repository.Querier) error {
		if err := q.AddTagToTask(ctx, repository.AddTagToTaskParams{TaskID: task.ID, TagID: tag3.ID}); err != nil {
			return err
		}
		return forcedErr
	})
	require.ErrorIs(t, err, forcedErr)

	// Verify rollback: task still only has 2 tags, tag3 was NOT persisted
	tagsAfterRollback, err := testStore.GetTagsForTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Len(t, tagsAfterRollback, 2)
	for _, tg := range tagsAfterRollback {
		assert.NotEqual(t, tag3.ID, tg.ID)
	}
}
