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

type mockTaskQuerier struct {
	mockQuerier
	createTaskFn      func(ctx context.Context, arg repository.CreateTaskParams) (repository.Task, error)
	getTaskByIDFn     func(ctx context.Context, id uuid.UUID) (repository.Task, error)
	listTasksFn       func(ctx context.Context, arg repository.ListTasksParams) ([]repository.Task, error)
	countTasksFn      func(ctx context.Context, arg repository.CountTasksParams) (int64, error)
	updateTaskFn      func(ctx context.Context, arg repository.UpdateTaskParams) (repository.Task, error)
	softDeleteTaskFn  func(ctx context.Context, id uuid.UUID) error
	getProjectByIDFn  func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	getTagsForTaskFn  func(ctx context.Context, taskID uuid.UUID) ([]repository.Tag, error)
	getTagsForTasksFn func(ctx context.Context, taskIDs []uuid.UUID) ([]repository.GetTagsForTasksRow, error)
}

func (m *mockTaskQuerier) CreateTask(ctx context.Context, arg repository.CreateTaskParams) (repository.Task, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, arg)
	}
	return repository.Task{}, nil
}

func (m *mockTaskQuerier) GetTaskByID(ctx context.Context, id uuid.UUID) (repository.Task, error) {
	if m.getTaskByIDFn != nil {
		return m.getTaskByIDFn(ctx, id)
	}
	return repository.Task{}, pgx.ErrNoRows
}

func (m *mockTaskQuerier) ListTasks(ctx context.Context, arg repository.ListTasksParams) ([]repository.Task, error) {
	if m.listTasksFn != nil {
		return m.listTasksFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockTaskQuerier) CountTasks(ctx context.Context, arg repository.CountTasksParams) (int64, error) {
	if m.countTasksFn != nil {
		return m.countTasksFn(ctx, arg)
	}
	return 0, nil
}

func (m *mockTaskQuerier) UpdateTask(ctx context.Context, arg repository.UpdateTaskParams) (repository.Task, error) {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, arg)
	}
	return repository.Task{}, pgx.ErrNoRows
}

func (m *mockTaskQuerier) SoftDeleteTask(ctx context.Context, id uuid.UUID) error {
	if m.softDeleteTaskFn != nil {
		return m.softDeleteTaskFn(ctx, id)
	}
	return nil
}

func (m *mockTaskQuerier) GetProjectByID(ctx context.Context, id uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, id)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockTaskQuerier) GetTagsForTask(ctx context.Context, taskID uuid.UUID) ([]repository.Tag, error) {
	if m.getTagsForTaskFn != nil {
		return m.getTagsForTaskFn(ctx, taskID)
	}
	return nil, nil
}

func (m *mockTaskQuerier) GetTagsForTasks(ctx context.Context, taskIDs []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
	if m.getTagsForTasksFn != nil {
		return m.getTagsForTasksFn(ctx, taskIDs)
	}
	return nil, nil
}

func TestTaskService_CreateTask(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()

	t.Run("Success_WithProjectAndDefaults", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: id, OwnerID: userID}, nil
			},
			createTaskFn: func(_ context.Context, arg repository.CreateTaskParams) (repository.Task, error) {
				require.Equal(t, userID, arg.CreatorID)
				require.Equal(t, "todo", arg.Status)
				require.Equal(t, "medium", arg.Priority)
				return repository.Task{
					ID:        taskID,
					CreatorID: userID,
					Title:     arg.Title,
					Status:    arg.Status,
					Priority:  arg.Priority,
					Version:   1,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.CreateTask(context.Background(), userID, CreateTaskInput{
			ProjectID: &projectID,
			Title:     "Build Auth",
		})

		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, taskID, res.ID)
		require.Equal(t, "todo", res.Status)
		require.Equal(t, "medium", res.Priority)
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		svc := NewTaskService(&mockTaskQuerier{})
		_, err := svc.CreateTask(context.Background(), userID, CreateTaskInput{
			Title:  "Task",
			Status: "invalid_status",
		})
		require.ErrorIs(t, err, ErrInvalidTaskStatus)
	})

	t.Run("AntiIDOR_ProjectNotOwned", func(t *testing.T) {
		otherUser := uuid.New()
		mock := &mockTaskQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: id, OwnerID: otherUser}, nil
			},
		}

		svc := NewTaskService(mock)
		_, err := svc.CreateTask(context.Background(), userID, CreateTaskInput{
			ProjectID: &projectID,
			Title:     "Task",
		})
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}

func TestTaskService_GetTaskByID(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{
					ID:        id,
					CreatorID: userID,
					Title:     "Test Task",
					Status:    "in_progress",
					Priority:  "high",
					Version:   1,
				}, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.GetTaskByID(context.Background(), userID, taskID)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, "Test Task", res.Title)
	})

	t.Run("AntiIDOR_NotAllowed", func(t *testing.T) {
		otherUser := uuid.New()
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{
					ID:        id,
					CreatorID: otherUser,
				}, nil
			},
		}

		svc := NewTaskService(mock)
		_, err := svc.GetTaskByID(context.Background(), userID, taskID)
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestTaskService_UpdateTask(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Conflict_OptimisticLock", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{
					ID:        id,
					CreatorID: userID,
					Title:     "Task",
					Version:   2, // DB version is 2
				}, nil
			},
		}

		svc := NewTaskService(mock)
		_, err := svc.UpdateTask(context.Background(), userID, taskID, UpdateTaskInput{
			Title:   "Concurrent edit",
			Version: 1, // Client has version 1
		})
		require.ErrorIs(t, err, ErrOptimisticLockConflict)
	})

	t.Run("Success", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{
					ID:        id,
					CreatorID: userID,
					Title:     "Task",
					Status:    "todo",
					Priority:  "medium",
					Version:   1,
				}, nil
			},
			updateTaskFn: func(_ context.Context, arg repository.UpdateTaskParams) (repository.Task, error) {
				return repository.Task{
					ID:        arg.ID,
					CreatorID: userID,
					Title:     arg.Title,
					Status:    arg.Status,
					Priority:  arg.Priority,
					Version:   2,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.UpdateTask(context.Background(), userID, taskID, UpdateTaskInput{
			Title:   "Updated Task",
			Status:  "completed",
			Version: 1,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, int32(2), res.Version)
		require.Equal(t, "completed", res.Status)
	})
}

func TestTaskService_DeleteTask(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		deleted := false
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{ID: id, CreatorID: userID}, nil
			},
			softDeleteTaskFn: func(_ context.Context, id uuid.UUID) error {
				if id == taskID {
					deleted = true
				}
				return nil
			},
		}

		svc := NewTaskService(mock)
		err := svc.DeleteTask(context.Background(), userID, taskID)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}
