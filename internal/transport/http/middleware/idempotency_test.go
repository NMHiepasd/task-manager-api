package middleware_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/token"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIdempotencyQuerier simulates idempotency database storage in memory.
type mockIdempotencyQuerier struct {
	mu      sync.Mutex
	records map[string]repository.IdempotencyKey
}

func newMockIdempotencyQuerier() *mockIdempotencyQuerier {
	return &mockIdempotencyQuerier{
		records: make(map[string]repository.IdempotencyKey),
	}
}

func (m *mockIdempotencyQuerier) recordKey(userID uuid.UUID, key string) string {
	return userID.String() + ":" + key
}

func (m *mockIdempotencyQuerier) CreateIdempotencyKey(_ context.Context, arg repository.CreateIdempotencyKeyParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := m.recordKey(arg.UserID, arg.Key)
	if _, exists := m.records[k]; exists {
		return &pgconn.PgError{Code: "23505"} // duplicate key error
	}

	m.records[k] = repository.IdempotencyKey{
		Key:         arg.Key,
		UserID:      arg.UserID,
		RequestPath: arg.RequestPath,
		ParamsHash:  arg.ParamsHash,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		LockedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	return nil
}

func (m *mockIdempotencyQuerier) GetIdempotencyKey(_ context.Context, arg repository.GetIdempotencyKeyParams) (repository.IdempotencyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := m.recordKey(arg.UserID, arg.Key)
	rec, exists := m.records[k]
	if !exists {
		return repository.IdempotencyKey{}, pgx.ErrNoRows
	}
	return rec, nil
}

func (m *mockIdempotencyQuerier) UpdateIdempotencyKeyResponse(_ context.Context, arg repository.UpdateIdempotencyKeyResponseParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := m.recordKey(arg.UserID, arg.Key)
	rec, exists := m.records[k]
	if !exists {
		return pgx.ErrNoRows
	}
	rec.ResponseCode = arg.ResponseCode
	rec.ResponseBody = arg.ResponseBody
	rec.LockedAt = pgtype.Timestamptz{Valid: false}
	m.records[k] = rec
	return nil
}

// Stubs to fulfill repository.Querier
func (m *mockIdempotencyQuerier) DeleteOldIdempotencyKeys(context.Context, pgtype.Timestamptz) error {
	return nil
}

func (m *mockIdempotencyQuerier) ListTasksDueSoon(context.Context, repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) CreateUser(context.Context, repository.CreateUserParams) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockIdempotencyQuerier) GetUserByEmail(context.Context, string) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockIdempotencyQuerier) GetUserByID(context.Context, uuid.UUID) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockIdempotencyQuerier) CreateRefreshToken(context.Context, repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
	return repository.RefreshToken{}, nil
}

func (m *mockIdempotencyQuerier) GetRefreshTokenByHash(context.Context, string) (repository.RefreshToken, error) {
	return repository.RefreshToken{}, nil
}

func (m *mockIdempotencyQuerier) RevokeRefreshToken(context.Context, string) error { return nil }
func (m *mockIdempotencyQuerier) RevokeAllUserRefreshTokens(context.Context, uuid.UUID) error {
	return nil
}

func (m *mockIdempotencyQuerier) DeleteExpiredRefreshTokens(context.Context) error { return nil }
func (m *mockIdempotencyQuerier) CreateProject(context.Context, repository.CreateProjectParams) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockIdempotencyQuerier) GetProjectByID(context.Context, uuid.UUID) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockIdempotencyQuerier) ListProjectsByOwner(context.Context, repository.ListProjectsByOwnerParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListProjectsKeyset(context.Context, repository.ListProjectsKeysetParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) CountProjectsByOwner(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockIdempotencyQuerier) UpdateProject(context.Context, repository.UpdateProjectParams) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockIdempotencyQuerier) SoftDeleteProject(context.Context, repository.SoftDeleteProjectParams) error {
	return nil
}

func (m *mockIdempotencyQuerier) CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockIdempotencyQuerier) GetTaskByID(context.Context, uuid.UUID) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockIdempotencyQuerier) ListTasks(context.Context, repository.ListTasksParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListTasksKeyset(context.Context, repository.ListTasksKeysetParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) CountTasks(context.Context, repository.CountTasksParams) (int64, error) {
	return 0, nil
}

func (m *mockIdempotencyQuerier) UpdateTask(context.Context, repository.UpdateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockIdempotencyQuerier) SoftDeleteTask(context.Context, uuid.UUID) error { return nil }
func (m *mockIdempotencyQuerier) CreateTag(context.Context, repository.CreateTagParams) (repository.Tag, error) {
	return repository.Tag{}, nil
}

func (m *mockIdempotencyQuerier) GetTagByID(context.Context, uuid.UUID) (repository.Tag, error) {
	return repository.Tag{}, nil
}

func (m *mockIdempotencyQuerier) ListTagsByUser(context.Context, uuid.UUID) ([]repository.Tag, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) DeleteTag(context.Context, repository.DeleteTagParams) error {
	return nil
}

func (m *mockIdempotencyQuerier) AddTagToTask(context.Context, repository.AddTagToTaskParams) error {
	return nil
}

func (m *mockIdempotencyQuerier) RemoveTagFromTask(context.Context, repository.RemoveTagFromTaskParams) error {
	return nil
}

func (m *mockIdempotencyQuerier) ClearTaskTags(context.Context, uuid.UUID) error { return nil }
func (m *mockIdempotencyQuerier) GetTagsForTask(context.Context, uuid.UUID) ([]repository.Tag, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) GetTagsForTasks(context.Context, []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListTasksByCreator(context.Context, repository.ListTasksByCreatorParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListUsers(context.Context, repository.ListUsersParams) ([]repository.User, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) SoftDeleteUser(context.Context, uuid.UUID) error { return nil }
func (m *mockIdempotencyQuerier) UpdateUser(context.Context, repository.UpdateUserParams) (repository.User, error) {
	return repository.User{}, nil
}

func (m *mockIdempotencyQuerier) AddProjectMember(context.Context, repository.AddProjectMemberParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}

func (m *mockIdempotencyQuerier) CountProjectMembers(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockIdempotencyQuerier) CountProjectsByMember(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockIdempotencyQuerier) DeleteProjectMember(context.Context, repository.DeleteProjectMemberParams) error {
	return nil
}

func (m *mockIdempotencyQuerier) GetProjectMember(context.Context, repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}

func (m *mockIdempotencyQuerier) GetProjectMemberWithUser(context.Context, repository.GetProjectMemberWithUserParams) (repository.GetProjectMemberWithUserRow, error) {
	return repository.GetProjectMemberWithUserRow{}, nil
}

func (m *mockIdempotencyQuerier) ListProjectMembers(context.Context, uuid.UUID) ([]repository.ListProjectMembersRow, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListProjectsByMember(context.Context, repository.ListProjectsByMemberParams) ([]repository.ListProjectsByMemberRow, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) ListProjectsByMemberKeyset(context.Context, repository.ListProjectsByMemberKeysetParams) ([]repository.ListProjectsByMemberKeysetRow, error) {
	return nil, nil
}

func (m *mockIdempotencyQuerier) UpdateProjectMemberRole(context.Context, repository.UpdateProjectMemberRoleParams) (repository.ProjectMember, error) {
	return repository.ProjectMember{}, nil
}

func setupIdempotencyTestRouter(mock *mockIdempotencyQuerier, testUserID uuid.UUID, handlerCallCounter *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Inject auth user
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, testUserID)
		c.Set(middleware.AuthorizationPayloadKey, &token.UserClaims{
			UserID: testUserID,
			Role:   "user",
		})
		c.Next()
	})

	router.Use(middleware.Idempotency(mock))

	router.POST("/api/v1/tasks", func(c *gin.Context) {
		*handlerCallCounter++
		c.JSON(http.StatusCreated, gin.H{
			"id":     "11111111-1111-1111-1111-111111111111",
			"status": "created",
		})
	})

	return router
}

func TestIdempotency_NoHeader(t *testing.T) {
	mock := newMockIdempotencyQuerier()
	count := 0
	router := setupIdempotencyTestRouter(mock, uuid.New(), &count)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"title":"Task"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, 1, count)
	assert.Empty(t, rec.Header().Get("Idempotent-Replayed"))
}

func TestIdempotency_InvalidLength(t *testing.T) {
	mock := newMockIdempotencyQuerier()
	count := 0
	router := setupIdempotencyTestRouter(mock, uuid.New(), &count)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"title":"Task"}`))
	req.Header.Set("Idempotency-Key", strings.Repeat("a", 256))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_IDEMPOTENCY_KEY")
	assert.Equal(t, 0, count)
}

func TestIdempotency_FirstRequestAndReplay(t *testing.T) {
	mock := newMockIdempotencyQuerier()
	count := 0
	userID := uuid.New()
	router := setupIdempotencyTestRouter(mock, userID, &count)

	key := uuid.NewString()
	body := `{"title":"Unique Task"}`

	// First request: should execute handler and return 201 Created
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(body))
	req1.Header.Set("Idempotency-Key", key)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	assert.Equal(t, http.StatusCreated, rec1.Code)
	assert.Equal(t, 1, count)
	assert.Empty(t, rec1.Header().Get("Idempotent-Replayed"))
	assert.Contains(t, rec1.Body.String(), "11111111-1111-1111-1111-111111111111")

	// Second request with SAME key and SAME body: must return cached response without calling handler
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(body))
	req2.Header.Set("Idempotency-Key", key)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusCreated, rec2.Code)
	assert.Equal(t, 1, count) // Count remains 1! Handler was NOT executed again
	assert.Equal(t, "true", rec2.Header().Get("Idempotent-Replayed"))
	assert.Equal(t, rec1.Body.String(), rec2.Body.String())
}

func TestIdempotency_PayloadMismatch(t *testing.T) {
	mock := newMockIdempotencyQuerier()
	count := 0
	userID := uuid.New()
	router := setupIdempotencyTestRouter(mock, userID, &count)

	key := uuid.NewString()

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"title":"Original"}`))
	req1.Header.Set("Idempotency-Key", key)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusCreated, rec1.Code)
	require.Equal(t, 1, count)

	// Reusing SAME key with DIFFERENT body must be rejected
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"title":"Tampered"}`))
	req2.Header.Set("Idempotency-Key", key)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusConflict, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "IDEMPOTENCY_KEY_MISMATCH")
	assert.Equal(t, 1, count)
}

func TestIdempotency_ConcurrentInProgress(t *testing.T) {
	mock := newMockIdempotencyQuerier()
	count := 0
	userID := uuid.New()
	router := setupIdempotencyTestRouter(mock, userID, &count)

	key := uuid.NewString()
	body := `{"title":"In Flight"}`
	hash := sha256.Sum256([]byte(body))

	// Pre-seed an in-progress record (ResponseCode is null)
	err := mock.CreateIdempotencyKey(context.Background(), repository.CreateIdempotencyKeyParams{
		Key:         key,
		UserID:      userID,
		RequestPath: "/api/v1/tasks",
		ParamsHash:  hex.EncodeToString(hash[:]),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(body))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "IDEMPOTENCY_IN_PROGRESS")
	assert.Equal(t, 0, count)
}
