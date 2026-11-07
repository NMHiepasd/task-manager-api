package service

import (
	"context"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTaskQuerier struct {
	mockQuerier
	createTaskFn      func(ctx context.Context, arg repository.CreateTaskParams) (repository.Task, error)
	getTaskByIDFn     func(ctx context.Context, id uuid.UUID) (repository.Task, error)
	listTasksFn       func(ctx context.Context, arg repository.ListTasksParams) ([]repository.Task, error)
	listTasksKeysetFn func(ctx context.Context, arg repository.ListTasksKeysetParams) ([]repository.Task, error)
	countTasksFn      func(ctx context.Context, arg repository.CountTasksParams) (int64, error)
	updateTaskFn      func(ctx context.Context, arg repository.UpdateTaskParams) (repository.Task, error)
	softDeleteTaskFn  func(ctx context.Context, id uuid.UUID) error
	getProjectByIDFn  func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	getTagsForTaskFn  func(ctx context.Context, taskID uuid.UUID) ([]repository.Tag, error)
	getTagsForTasksFn func(ctx context.Context, taskIDs []uuid.UUID) ([]repository.GetTagsForTasksRow, error)
	clearTaskTagsFn   func(ctx context.Context, taskID uuid.UUID) error
	addTagToTaskFn    func(ctx context.Context, arg repository.AddTagToTaskParams) error
}

func (m *mockTaskQuerier) ClearTaskTags(ctx context.Context, taskID uuid.UUID) error {
	if m.clearTaskTagsFn != nil {
		return m.clearTaskTagsFn(ctx, taskID)
	}
	return nil
}

func (m *mockTaskQuerier) AddTagToTask(ctx context.Context, arg repository.AddTagToTaskParams) error {
	if m.addTagToTaskFn != nil {
		return m.addTagToTaskFn(ctx, arg)
	}
	return nil
}

func (m *mockTaskQuerier) ListTasksKeyset(ctx context.Context, arg repository.ListTasksKeysetParams) ([]repository.Task, error) {
	if m.listTasksKeysetFn != nil {
		return m.listTasksKeysetFn(ctx, arg)
	}
	return nil, nil
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

	t.Run("NotFound", func(t *testing.T) {
		mock := &mockTaskQuerier{}
		svc := NewTaskService(mock)
		err := svc.DeleteTask(context.Background(), userID, taskID)
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestTaskService_ListTasks(t *testing.T) {
	userID := uuid.New()
	taskID1 := uuid.New()
	taskID2 := uuid.New()

	t.Run("OffsetPagination_Success", func(t *testing.T) {
		mock := &mockTaskQuerier{
			countTasksFn: func(_ context.Context, _ repository.CountTasksParams) (int64, error) {
				return 2, nil
			},
			listTasksFn: func(_ context.Context, _ repository.ListTasksParams) ([]repository.Task, error) {
				return []repository.Task{
					{ID: taskID1, CreatorID: userID, Title: "Task 1", Status: "todo", Priority: "medium", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
					{ID: taskID2, CreatorID: userID, Title: "Task 2", Status: "done", Priority: "high", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
				}, nil
			},
			getTagsForTasksFn: func(_ context.Context, _ []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
				return nil, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.ListTasks(context.Background(), userID, ListTasksFilter{
			Page:  1,
			Limit: 10,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, int64(2), res.TotalItems)
		assert.Len(t, res.Items, 2)
	})

	t.Run("KeysetPagination_Success", func(t *testing.T) {
		mock := &mockTaskQuerier{
			listTasksKeysetFn: func(_ context.Context, _ repository.ListTasksKeysetParams) ([]repository.Task, error) {
				return []repository.Task{
					{ID: taskID1, CreatorID: userID, Title: "Task 1", Status: "todo", Priority: "medium", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
				}, nil
			},
			getTagsForTasksFn: func(_ context.Context, _ []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
				return nil, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.ListTasks(context.Background(), userID, ListTasksFilter{
			Cursor: "MjAyNi0xMS0wNlQxNjo1Mzo1My40NTAwMjVafDAxYTFiMWM3LWYyYWEtNzk5Zi1hY2FmLWMwNDM5OTZlOGE4OQ",
			Limit:  10,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Len(t, res.Items, 1)
	})
}

func TestTaskService_ListProjectTasks(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	t.Run("Success_UserOwnsProject", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: id, OwnerID: userID}, nil
			},
			countTasksFn: func(_ context.Context, _ repository.CountTasksParams) (int64, error) {
				return 1, nil
			},
			listTasksFn: func(_ context.Context, _ repository.ListTasksParams) ([]repository.Task, error) {
				return []repository.Task{
					{ID: uuid.New(), CreatorID: userID, Title: "Project Task 1", Status: "todo", Priority: "medium", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
				}, nil
			},
			getTagsForTasksFn: func(_ context.Context, _ []uuid.UUID) ([]repository.GetTagsForTasksRow, error) {
				return nil, nil
			},
		}

		svc := NewTaskService(mock)
		res, err := svc.ListProjectTasks(context.Background(), userID, projectID, ListTasksFilter{Limit: 10})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Len(t, res.Items, 1)
	})

	t.Run("AntiIDOR_ProjectNotOwned", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: id, OwnerID: uuid.New()}, nil // different owner
			},
		}

		svc := NewTaskService(mock)
		_, err := svc.ListProjectTasks(context.Background(), userID, projectID, ListTasksFilter{Limit: 10})
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}

func TestTaskService_SetTaskTags(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	tagID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock := &mockTaskQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				return repository.Task{ID: id, CreatorID: userID}, nil
			},
			clearTaskTagsFn: func(_ context.Context, _ uuid.UUID) error {
				return nil
			},
			addTagToTaskFn: func(_ context.Context, _ repository.AddTagToTaskParams) error {
				return nil
			},
			getTagsForTaskFn: func(_ context.Context, _ uuid.UUID) ([]repository.Tag, error) {
				return []repository.Tag{
					{ID: tagID, UserID: userID, Name: "backend", Color: "#3b82f6"},
				}, nil
			},
		}

		svc := NewTaskService(mock)
		tags, err := svc.SetTaskTags(context.Background(), userID, taskID, []uuid.UUID{tagID})
		require.NoError(t, err)
		require.Len(t, tags, 1)
		assert.Equal(t, "backend", tags[0].Name)
	})

	t.Run("NotFound", func(t *testing.T) {
		mock := &mockTaskQuerier{}
		svc := NewTaskService(mock)
		_, err := svc.SetTaskTags(context.Background(), userID, taskID, []uuid.UUID{tagID})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}
