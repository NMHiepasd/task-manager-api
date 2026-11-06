package handler

import (
	"errors"
	"net/http"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateProjectRequest defines input for creating a project.
type CreateProjectRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=150"`
	Description string `json:"description" binding:"max=1000"`
	Color       string `json:"color" binding:"omitempty,max=20"`
}

// UpdateProjectRequest defines input for updating a project.
type UpdateProjectRequest struct {
	Name        string `json:"name" binding:"omitempty,min=1,max=150"`
	Description string `json:"description" binding:"max=1000"`
	Color       string `json:"color" binding:"omitempty,max=20"`
	Version     int32  `json:"version" binding:"required,min=1"`
}

// ListProjectsQuery specifies query parameters for listing projects.
type ListProjectsQuery struct {
	Page  int32 `form:"page,default=1" binding:"omitempty,min=1"`
	Limit int32 `form:"limit,default=10" binding:"omitempty,min=1,max=100"`
}

// ProjectHandler handles HTTP requests for projects.
type ProjectHandler struct {
	projectService service.ProjectService
}

// NewProjectHandler constructs a new ProjectHandler.
func NewProjectHandler(projectService service.ProjectService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService}
}

// Create creates a new project owned by the authenticated user.
// POST /api/v1/projects
func (h *ProjectHandler) Create(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid project input", err.Error()))
		return
	}

	project, err := h.projectService.CreateProject(c.Request.Context(), userID, service.CreateProjectInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to create project"))
		return
	}

	c.JSON(http.StatusCreated, response.Success(project))
}

// GetByID retrieves project details by ID.
// GET /api/v1/projects/:id
func (h *ProjectHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid project UUID format"))
		return
	}

	project, err := h.projectService.GetProjectByID(c.Request.Context(), userID, projectID)
	if err != nil {
		if errors.Is(err, service.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, response.Error("PROJECT_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to get project"))
		return
	}

	c.JSON(http.StatusOK, response.Success(project))
}

// List retrieves paginated projects for the authenticated user.
// GET /api/v1/projects
func (h *ProjectHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	var query ListProjectsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid pagination query", err.Error()))
		return
	}

	result, err := h.projectService.ListProjects(c.Request.Context(), userID, service.PaginationParams{
		Page:  query.Page,
		Limit: query.Limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to list projects"))
		return
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// Update updates an existing project with optimistic concurrency control.
// PUT /api/v1/projects/:id
func (h *ProjectHandler) Update(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid project UUID format"))
		return
	}

	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid update project input", err.Error()))
		return
	}

	updated, err := h.projectService.UpdateProject(c.Request.Context(), userID, projectID, service.UpdateProjectInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		Version:     req.Version,
	})
	if err != nil {
		if errors.Is(err, service.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, response.Error("PROJECT_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		if errors.Is(err, service.ErrOptimisticLockConflict) {
			c.JSON(http.StatusConflict, response.Error("OPTIMISTIC_LOCK_CONFLICT", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to update project"))
		return
	}

	c.JSON(http.StatusOK, response.Success(updated))
}

// Delete soft-deletes a project owned by the user.
// DELETE /api/v1/projects/:id
func (h *ProjectHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid project UUID format"))
		return
	}

	if err := h.projectService.DeleteProject(c.Request.Context(), userID, projectID); err != nil {
		if errors.Is(err, service.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, response.Error("PROJECT_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to delete project"))
		return
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"message": "Project deleted successfully",
	}))
}
