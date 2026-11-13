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

type mockProjectService struct {
	createProjectFn    func(ctx context.Context, ownerID uuid.UUID, input service.CreateProjectInput) (*service.ProjectResponse, error)
	getProjectByIDFn   func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) (*service.ProjectResponse, error)
	listProjectsFn     func(ctx context.Context, ownerID uuid.UUID, pagination service.PaginationParams) (*service.PaginatedProjectsResponse, error)
	updateProjectFn    func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID, input service.UpdateProjectInput) (*service.ProjectResponse, error)
	deleteProjectFn    func(ctx context.Context, ownerID uuid.UUID, projectID uuid.UUID) error
	addMemberFn        func(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, input service.AddProjectMemberInput) (*service.ProjectMemberResponse, error)
	listMembersFn      func(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID) ([]service.ProjectMemberResponse, error)
	updateMemberRoleFn func(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID, input service.UpdateProjectMemberRoleInput) (*service.ProjectMemberResponse, error)
	removeMemberFn     func(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID) error
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

func (m *mockProjectService) AddMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, input service.AddProjectMemberInput) (*service.ProjectMemberResponse, error) {
	if m.addMemberFn != nil {
		return m.addMemberFn(ctx, currentUserID, projectID, input)
	}
	return nil, nil
}

func (m *mockProjectService) ListMembers(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID) ([]service.ProjectMemberResponse, error) {
	if m.listMembersFn != nil {
		return m.listMembersFn(ctx, currentUserID, projectID)
	}
	return nil, nil
}

func (m *mockProjectService) UpdateMemberRole(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID, input service.UpdateProjectMemberRoleInput) (*service.ProjectMemberResponse, error) {
	if m.updateMemberRoleFn != nil {
		return m.updateMemberRoleFn(ctx, currentUserID, projectID, targetUserID, input)
	}
	return nil, nil
}

func (m *mockProjectService) RemoveMember(ctx context.Context, currentUserID uuid.UUID, projectID uuid.UUID, targetUserID uuid.UUID) error {
	if m.removeMemberFn != nil {
		return m.removeMemberFn(ctx, currentUserID, projectID, targetUserID)
	}
	return nil
}

func setupTestProjectRouter(handler *ProjectHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(nil))
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})

	router.POST("/projects", handler.Create)
	router.GET("/projects", handler.List)
	router.GET("/projects/:id", handler.GetByID)
	router.PUT("/projects/:id", handler.Update)
	router.DELETE("/projects/:id", handler.Delete)
	router.POST("/projects/:id/members", handler.AddMember)
	router.GET("/projects/:id/members", handler.ListMembers)
	router.PATCH("/projects/:id/members/:userId", handler.UpdateMemberRole)
	router.DELETE("/projects/:id/members/:userId", handler.RemoveMember)

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

	t.Run("BadRequest_MissingName", func(t *testing.T) {
		handler := NewProjectHandler(&mockProjectService{})
		router := setupTestProjectRouter(handler, userID)

		body := map[string]string{"color": "#ffffff"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/projects", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestProjectHandler_GetByID(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockProjectService{
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID, pID uuid.UUID) (*service.ProjectResponse, error) {
				return &service.ProjectResponse{
					ID:        pID,
					OwnerID:   userID,
					Name:      "Test Project",
					Version:   1,
					CreatedAt: time.Now(),
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
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestProjectHandler_Update(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	t.Run("Conflict_409_OptimisticLock", func(t *testing.T) {
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
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
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

func TestProjectHandler_Members(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()
	targetUserID := uuid.New()

	t.Run("AddMember_201", func(t *testing.T) {
		mock := &mockProjectService{
			addMemberFn: func(_ context.Context, _ uuid.UUID, pID uuid.UUID, input service.AddProjectMemberInput) (*service.ProjectMemberResponse, error) {
				return &service.ProjectMemberResponse{
					ID:        uuid.New(),
					ProjectID: pID,
					UserID:    targetUserID,
					Email:     input.Email,
					FullName:  "Test Collaborator",
					Role:      input.Role,
					CreatedAt: time.Now(),
				}, nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		body := map[string]string{
			"email": "collab@example.com",
			"role":  "editor",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/projects/"+projectID.String()+"/members", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("ListMembers_200", func(t *testing.T) {
		mock := &mockProjectService{
			listMembersFn: func(_ context.Context, _ uuid.UUID, pID uuid.UUID) ([]service.ProjectMemberResponse, error) {
				return []service.ProjectMemberResponse{
					{ID: uuid.New(), ProjectID: pID, UserID: userID, Role: "owner", Email: "owner@example.com"},
				}, nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		req, _ := http.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/members", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("UpdateMemberRole_200", func(t *testing.T) {
		mock := &mockProjectService{
			updateMemberRoleFn: func(_ context.Context, _ uuid.UUID, pID uuid.UUID, tID uuid.UUID, input service.UpdateProjectMemberRoleInput) (*service.ProjectMemberResponse, error) {
				return &service.ProjectMemberResponse{
					ID:        uuid.New(),
					ProjectID: pID,
					UserID:    tID,
					Role:      input.Role,
				}, nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		body := map[string]string{"role": "viewer"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPatch, "/projects/"+projectID.String()+"/members/"+targetUserID.String(), bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("RemoveMember_200", func(t *testing.T) {
		mock := &mockProjectService{
			removeMemberFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) error {
				return nil
			},
		}

		handler := NewProjectHandler(mock)
		router := setupTestProjectRouter(handler, userID)

		req, _ := http.NewRequest(http.MethodDelete, "/projects/"+projectID.String()+"/members/"+targetUserID.String(), nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
