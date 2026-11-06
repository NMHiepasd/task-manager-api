package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mockTaskService struct {
	createTaskFn  func(ctx context.Context, userID uuid.UUID, input service.CreateTaskInput) (*service.TaskResponse, error)
	getTaskByIDFn func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*service.TaskResponse, error)
	listTasksFn   func(ctx context.Context, userID uuid.UUID, filter service.ListTasksFilter) (*service.PaginatedTasksResponse, error)
	updateTaskFn  func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input service.UpdateTaskInput) (*service.TaskResponse, error)
	deleteTaskFn  func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) error
}

func (m *mockTaskService) CreateTask(ctx context.Context, userID uuid.UUID, input service.CreateTaskInput) (*service.TaskResponse, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, userID, input)
	}
	return nil, nil
}

func (m *mockTaskService) GetTaskByID(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*service.TaskResponse, error) {
	if m.getTaskByIDFn != nil {
		return m.getTaskByIDFn(ctx, userID, taskID)
	}
	return nil, nil
}

func (m *mockTaskService) ListTasks(ctx context.Context, userID uuid.UUID, filter service.ListTasksFilter) (*service.PaginatedTasksResponse, error) {
	if m.listTasksFn != nil {
		return m.listTasksFn(ctx, userID, filter)
	}
	return nil, nil
}

func (m *mockTaskService) UpdateTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input service.UpdateTaskInput) (*service.TaskResponse, error) {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, userID, taskID, input)
	}
	return nil, nil
}

func (m *mockTaskService) DeleteTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) error {
	if m.deleteTaskFn != nil {
		return m.deleteTaskFn(ctx, userID, taskID)
	}
	return nil
}

func setupTestTaskRouter(h *TaskHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})
	r.POST("/tasks", h.Create)
	r.GET("/tasks", h.List)
	r.GET("/tasks/:id", h.GetByID)
	r.PUT("/tasks/:id", h.Update)
	r.DELETE("/tasks/:id", h.Delete)
	return r
}

func TestTaskHandler_Create(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Created_201", func(t *testing.T) {
		mock := &mockTaskService{
			createTaskFn: func(_ context.Context, uID uuid.UUID, input service.CreateTaskInput) (*service.TaskResponse, error) {
				return &service.TaskResponse{
					ID:        taskID,
					CreatorID: uID,
					Title:     input.Title,
					Status:    "todo",
					Priority:  "medium",
					Version:   1,
					CreatedAt: time.Now(),
				}, nil
			},
		}

		h := NewTaskHandler(mock)
		router := setupTestTaskRouter(h, userID)

		body := map[string]string{"title": "Implement feature"}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(b))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("BadRequest_MissingTitle", func(t *testing.T) {
		h := NewTaskHandler(&mockTaskService{})
		router := setupTestTaskRouter(h, userID)

		body := map[string]string{"description": "No title"}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(b))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestTaskHandler_GetByID(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockTaskService{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID, id uuid.UUID) (*service.TaskResponse, error) {
				return &service.TaskResponse{
					ID:      id,
					Title:   "Task 1",
					Version: 1,
				}, nil
			},
		}

		h := NewTaskHandler(mock)
		router := setupTestTaskRouter(h, userID)

		req, _ := http.NewRequest(http.MethodGet, "/tasks/"+taskID.String(), nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestTaskHandler_Update_Conflict(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	mock := &mockTaskService{
		updateTaskFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ service.UpdateTaskInput) (*service.TaskResponse, error) {
			return nil, service.ErrOptimisticLockConflict
		},
	}

	h := NewTaskHandler(mock)
	router := setupTestTaskRouter(h, userID)

	body := map[string]interface{}{
		"title":   "Concurrent Edit",
		"version": 1,
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPut, "/tasks/"+taskID.String(), bytes.NewReader(b))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}
