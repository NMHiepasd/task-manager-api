package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mockProjectService struct {
	createProjectFn  func(ctx context.Context, ownerID uuid.UUID, input service.CreateProjectInput) (*service.ProjectResponse, error)
	getProjectByIDFn func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) (*service.ProjectResponse, error)
	listProjectsFn   func(ctx context.Context, ownerID uuid.UUID, pagination service.PaginationParams) (*service.PaginatedProjectsResponse, error)
	updateProjectFn  func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input service.UpdateProjectInput) (*service.ProjectResponse, error)
	deleteProjectFn  func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error
}

func (m *mockProjectService) CreateProject(ctx context.Context, ownerID uuid.UUID, input service.CreateProjectInput) (*service.ProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, ownerID, input)
	}
	return nil, nil
}

func (m *mockProjectService) GetProjectByID(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) (*service.ProjectResponse, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, ownerID, projectID)
	}
	return nil, nil
}

func (m *mockProjectService) ListProjects(ctx context.Context, ownerID uuid.UUID, pagination service.PaginationParams) (*service.PaginatedProjectsResponse, error) {
	if m.listProjectsFn != nil {
		return m.listProjectsFn(ctx, ownerID, pagination)
	}
	return nil, nil
}

func (m *mockProjectService) UpdateProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input service.UpdateProjectInput) (*service.ProjectResponse, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, ownerID, projectID, input)
	}
	return nil, nil
}

func (m *mockProjectService) DeleteProject(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error {
	if m.deleteProjectFn != nil {
		return m.deleteProjectFn(ctx, ownerID, projectID)
	}
	return nil
}

func setupTestProjectRouter(handler *ProjectHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})

	router.POST("/projects", handler.Create)
	router.GET("/projects", handler.List)
	router.GET("/projects/:id", handler.GetByID)
	router.PUT("/projects/:id", handler.Update)
	router.DELETE("/projects/:id", handler.Delete)

	return router
}

func TestProjectHandler_Create(t *testing.T) {
	userID := uuid.New()

	t.Run("Created_201", func(t *testing.T) {
		mock := &mockProjectService{
			createProjectFn: func(_ context.Context, oID uuid.UUID, input service.CreateProjectInput) (*service.ProjectResponse, error) {
				return &service.ProjectResponse{
					ID:        uuid.New(),
					OwnerID:   oID,
					Name:      input.Name,
					Version:   1,
					CreatedAt: time.Now(),
				}, nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		body := map[string]string{"name": "New Project"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/projects", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("BadRequest_400_MissingName", func(t *testing.T) {
		handler := NewProjectHandler(&mockProjectService{})
		router := setupTestProjectRouter(handler, userID)

		body := map[string]string{"description": "No name provided"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/projects", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestProjectHandler_GetByID(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockProjectService{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID, id uuid.UUID) (*service.ProjectResponse, error) {
				return &service.ProjectResponse{
					ID:      id,
					OwnerID: userID,
					Name:    "Alpha",
					Version: 1,
				}, nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		req, _ := http.NewRequest(http.MethodGet, "/projects/"+projectID.String(), nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("NotFound_404", func(t *testing.T) {
		mock := &mockProjectService{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*service.ProjectResponse, error) {
				return nil, service.ErrProjectNotFound
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		req, _ := http.NewRequest(http.MethodGet, "/projects/"+projectID.String(), nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestProjectHandler_Update_OptimisticLock(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	t.Run("Conflict_409", func(t *testing.T) {
		mock := &mockProjectService{
			updateProjectFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ service.UpdateProjectInput) (*service.ProjectResponse, error) {
				return nil, service.ErrOptimisticLockConflict
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		body := map[string]interface{}{
			"name":    "Concurrent Update",
			"version": 1,
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPut, "/projects/"+projectID.String(), bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestProjectHandler_Delete(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	mock := &mockProjectService{
		deleteProjectFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			return nil
		},
	}

	handler := NewProjectHandler(mock)
	router := setupTestProjectRouter(handler, userID)

	req, _ := http.NewRequest(http.MethodDelete, "/projects/"+projectID.String(), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
