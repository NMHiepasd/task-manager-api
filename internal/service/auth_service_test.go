package service

import (
	"context"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/hasher"
	"task-manager-api/pkg/token"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockQuerier struct {
	createUserFn            func(ctx context.Context, arg repository.CreateUserParams) (repository.User, error)
	getUserByEmailFn        func(ctx context.Context, lower string) (repository.User, error)
	getUserByIDFn           func(ctx context.Context, id uuid.UUID) (repository.User, error)
	createRefreshTokenFn    func(ctx context.Context, arg repository.CreateRefreshTokenParams) (repository.RefreshToken, error)
	getRefreshTokenByHashFn func(ctx context.Context, tokenHash string) (repository.RefreshToken, error)
	revokeRefreshTokenFn    func(ctx context.Context, tokenHash string) error
	revokeAllUserTokensFn   func(ctx context.Context, userID uuid.UUID) error
}

func (m *mockQuerier) CreateUser(ctx context.Context, arg repository.CreateUserParams) (repository.User, error) {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, arg)
	}
	return repository.User{}, nil
}

func (m *mockQuerier) GetUserByEmail(ctx context.Context, lower string) (repository.User, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, lower)
	}
	return repository.User{}, pgx.ErrNoRows
}

func (m *mockQuerier) GetUserByID(ctx context.Context, id uuid.UUID) (repository.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}
	return repository.User{}, pgx.ErrNoRows
}

func (m *mockQuerier) CreateRefreshToken(ctx context.Context, arg repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
	if m.createRefreshTokenFn != nil {
		return m.createRefreshTokenFn(ctx, arg)
	}
	return repository.RefreshToken{}, nil
}

func (m *mockQuerier) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (repository.RefreshToken, error) {
	if m.getRefreshTokenByHashFn != nil {
		return m.getRefreshTokenByHashFn(ctx, tokenHash)
	}
	return repository.RefreshToken{}, pgx.ErrNoRows
}

func (m *mockQuerier) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if m.revokeRefreshTokenFn != nil {
		return m.revokeRefreshTokenFn(ctx, tokenHash)
	}
	return nil
}

func (m *mockQuerier) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllUserTokensFn != nil {
		return m.revokeAllUserTokensFn(ctx, userID)
	}
	return nil
}

// Stubs for remaining Querier interface methods
func (m *mockQuerier) CountProjectsByOwner(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockQuerier) CreateProject(context.Context, repository.CreateProjectParams) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockQuerier) GetProjectByID(context.Context, uuid.UUID) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockQuerier) ListProjectsByOwner(context.Context, repository.ListProjectsByOwnerParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockQuerier) SoftDeleteProject(context.Context, repository.SoftDeleteProjectParams) error {
	return nil
}

func (m *mockQuerier) UpdateProject(context.Context, repository.UpdateProjectParams) (repository.Project, error) {
	return repository.Project{}, nil
}

func (m *mockQuerier) CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}
func (m *mockQuerier) DeleteExpiredRefreshTokens(context.Context) error { return nil }
func (m *mockQuerier) GetTaskByID(context.Context, uuid.UUID) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockQuerier) ListTasksByCreator(context.Context, repository.ListTasksByCreatorParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerier) ListUsers(context.Context, repository.ListUsersParams) ([]repository.User, error) {
	return nil, nil
}
func (m *mockQuerier) SoftDeleteTask(context.Context, uuid.UUID) error { return nil }
func (m *mockQuerier) SoftDeleteUser(context.Context, uuid.UUID) error { return nil }
func (m *mockQuerier) UpdateTask(context.Context, repository.UpdateTaskParams) (repository.Task, error) {
	return repository.Task{}, nil
}

func (m *mockQuerier) AddTagToTask(context.Context, repository.AddTagToTaskParams) error { return nil }

func (m *mockQuerier) ClearTaskTags(context.Context, uuid.UUID) error { return nil }

func (m *mockQuerier) CountTasks(context.Context, repository.CountTasksParams) (int64, error) {
	return 0, nil
}

func (m *mockQuerier) CreateTag(context.Context, repository.CreateTagParams) (repository.Tag, error) {
	return repository.Tag{}, nil
}
func (m *mockQuerier) DeleteTag(context.Context, repository.DeleteTagParams) error { return nil }
func (m *mockQuerier) GetTagByID(context.Context, uuid.UUID) (repository.Tag, error) {
	return repository.Tag{}, nil
}

func (m *mockQuerier) GetTagsForTask(context.Context, uuid.UUID) ([]repository.Tag, error) {
	return nil, nil
}

func (m *mockQuerier) GetTagsForTasks(context.Context, []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
	return nil, nil
}

func (m *mockQuerier) ListTagsByUser(context.Context, uuid.UUID) ([]repository.Tag, error) {
	return nil, nil
}

func (m *mockQuerier) ListTasks(context.Context, repository.ListTasksParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerier) ListProjectsKeyset(context.Context, repository.ListProjectsKeysetParams) ([]repository.Project, error) {
	return nil, nil
}

func (m *mockQuerier) ListTasksKeyset(context.Context, repository.ListTasksKeysetParams) ([]repository.Task, error) {
	return nil, nil
}

func (m *mockQuerier) RemoveTagFromTask(context.Context, repository.RemoveTagFromTaskParams) error {
	return nil
}

func (m *mockQuerier) UpdateUser(context.Context, repository.UpdateUserParams) (repository.User, error) {
	return repository.User{}, nil
}

var _ repository.Querier = (*mockQuerier)(nil)

func setupTestAuthService(t *testing.T, mock *mockQuerier) AuthService {
	h := hasher.NewBcryptHasher(bcrypt.MinCost)
	tm, err := token.NewJWTMaker("12345678901234567890123456789012")
	require.NoError(t, err)

	return NewAuthService(mock, h, tm, 15*time.Minute, 7*24*time.Hour)
}

func TestAuthService_Register_Success(t *testing.T) {
	userID := uuid.New()
	mock := &mockQuerier{
		getUserByEmailFn: func(_ context.Context, _ string) (repository.User, error) {
			return repository.User{}, pgx.ErrNoRows
		},
		createUserFn: func(_ context.Context, arg repository.CreateUserParams) (repository.User, error) {
			return repository.User{
				ID:        userID,
				Email:     arg.Email,
				FullName:  arg.FullName,
				Role:      arg.Role,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		createRefreshTokenFn: func(_ context.Context, arg repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
			return repository.RefreshToken{
				ID:        uuid.New(),
				UserID:    arg.UserID,
				TokenHash: arg.TokenHash,
			}, nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.Register(context.Background(), RegisterInput{
		Email:    "Alice@Example.com",
		Password: "Password123!",
		FullName: "Alice Doe",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.AccessToken)
	require.NotEmpty(t, res.RefreshToken)
	require.Equal(t, "alice@example.com", res.User.Email)
}

func TestAuthService_Register_EmailAlreadyExists(t *testing.T) {
	mock := &mockQuerier{
		getUserByEmailFn: func(_ context.Context, lower string) (repository.User, error) {
			return repository.User{
				ID:    uuid.New(),
				Email: lower,
			}, nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.Register(context.Background(), RegisterInput{
		Email:    "existing@example.com",
		Password: "Password123!",
		FullName: "Bob",
	})

	require.ErrorIs(t, err, ErrEmailAlreadyExists)
	require.Nil(t, res)
}

func TestAuthService_Login_Success(t *testing.T) {
	h := hasher.NewBcryptHasher(bcrypt.MinCost)
	hashed, err := h.HashPassword("CorrectPassword123!")
	require.NoError(t, err)

	userID := uuid.New()
	mock := &mockQuerier{
		getUserByEmailFn: func(_ context.Context, lower string) (repository.User, error) {
			return repository.User{
				ID:           userID,
				Email:        lower,
				PasswordHash: hashed,
				FullName:     "Charlie",
				Role:         "user",
				CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		createRefreshTokenFn: func(_ context.Context, arg repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
			return repository.RefreshToken{
				ID:     uuid.New(),
				UserID: arg.UserID,
			}, nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.Login(context.Background(), LoginInput{
		Email:    "Charlie@example.com",
		Password: "CorrectPassword123!",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.AccessToken)
	require.NotEmpty(t, res.RefreshToken)
	require.Equal(t, userID, res.User.ID)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	h := hasher.NewBcryptHasher(bcrypt.MinCost)
	hashed, err := h.HashPassword("CorrectPassword123!")
	require.NoError(t, err)

	mock := &mockQuerier{
		getUserByEmailFn: func(_ context.Context, lower string) (repository.User, error) {
			if lower == "unknown@example.com" {
				return repository.User{}, pgx.ErrNoRows
			}
			return repository.User{
				ID:           uuid.New(),
				Email:        lower,
				PasswordHash: hashed,
			}, nil
		},
	}

	authService := setupTestAuthService(t, mock)

	// Case 1: user does not exist
	_, err = authService.Login(context.Background(), LoginInput{
		Email:    "unknown@example.com",
		Password: "CorrectPassword123!",
	})
	require.ErrorIs(t, err, ErrInvalidCredentials)

	// Case 2: wrong password
	_, err = authService.Login(context.Background(), LoginInput{
		Email:    "known@example.com",
		Password: "WrongPassword!",
	})
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestAuthService_RefreshToken_Rotation(t *testing.T) {
	userID := uuid.New()
	user := repository.User{
		ID:        userID,
		Email:     "user@example.com",
		FullName:  "Test User",
		Role:      "user",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	revokedCount := 0
	mock := &mockQuerier{
		getRefreshTokenByHashFn: func(_ context.Context, tokenHash string) (repository.RefreshToken, error) {
			return repository.RefreshToken{
				ID:        uuid.New(),
				UserID:    userID,
				TokenHash: tokenHash,
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
				RevokedAt: pgtype.Timestamptz{Valid: false}, // not revoked
			}, nil
		},
		revokeRefreshTokenFn: func(_ context.Context, _ string) error {
			revokedCount++
			return nil
		},
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (repository.User, error) {
			return user, nil
		},
		createRefreshTokenFn: func(_ context.Context, arg repository.CreateRefreshTokenParams) (repository.RefreshToken, error) {
			return repository.RefreshToken{ID: uuid.New(), UserID: arg.UserID}, nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.RefreshToken(context.Background(), "some_plain_token_string")

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.AccessToken)
	require.NotEmpty(t, res.RefreshToken)
	require.Equal(t, 1, revokedCount) // Old token was rotated (revoked)
}

func TestAuthService_RefreshToken_ReuseDetection(t *testing.T) {
	userID := uuid.New()
	allRevoked := false

	mock := &mockQuerier{
		getRefreshTokenByHashFn: func(_ context.Context, tokenHash string) (repository.RefreshToken, error) {
			return repository.RefreshToken{
				ID:        uuid.New(),
				UserID:    userID,
				TokenHash: tokenHash,
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
				RevokedAt: pgtype.Timestamptz{Time: time.Now().Add(-10 * time.Minute), Valid: true}, // ALREADY REVOKED!
			}, nil
		},
		revokeAllUserTokensFn: func(_ context.Context, uID uuid.UUID) error {
			if uID == userID {
				allRevoked = true
			}
			return nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.RefreshToken(context.Background(), "reused_token")

	require.ErrorIs(t, err, ErrTokenReuseDetected)
	require.Nil(t, res)
	require.True(t, allRevoked, "All user tokens must be revoked upon reuse detection")
}

func TestAuthService_RefreshToken_Expired(t *testing.T) {
	mock := &mockQuerier{
		getRefreshTokenByHashFn: func(_ context.Context, tokenHash string) (repository.RefreshToken, error) {
			return repository.RefreshToken{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				TokenHash: tokenHash,
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}, // Expired
				RevokedAt: pgtype.Timestamptz{Valid: false},
			}, nil
		},
	}

	authService := setupTestAuthService(t, mock)
	res, err := authService.RefreshToken(context.Background(), "expired_token")

	require.ErrorIs(t, err, ErrTokenExpired)
	require.Nil(t, res)
}

func TestAuthService_Logout(t *testing.T) {
	revoked := false
	mock := &mockQuerier{
		revokeRefreshTokenFn: func(_ context.Context, _ string) error {
			revoked = true
			return nil
		},
	}

	authService := setupTestAuthService(t, mock)
	err := authService.Logout(context.Background(), "some_token")
	require.NoError(t, err)
	require.True(t, revoked)
}
