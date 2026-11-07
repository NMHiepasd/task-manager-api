package handler

import (
	"net/http"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"

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
	Cursor string `form:"cursor"`
	Page   int32  `form:"page,default=1" binding:"omitempty,min=1"`
	Limit  int32  `form:"limit,default=10" binding:"omitempty,min=1,max=100"`
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
// @Summary      Create project
// @Description  Create a new project owned by the authenticated user
// @Tags         projects
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        payload body CreateProjectRequest true "Project payload"
// @Success      201 {object} service.ProjectResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Router       /api/v1/projects [post]
func (h *ProjectHandler) Create(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	project, err := h.projectService.CreateProject(c.Request.Context(), userID, service.CreateProjectInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, project)
}

// GetByID retrieves project details by ID.
// @Summary      Get project by ID
// @Description  Retrieve project details by its UUID (enforcing anti-IDOR 404)
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Success      200 {object} service.ProjectResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/projects/{id} [get]
func (h *ProjectHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	project, err := h.projectService.GetProjectByID(c.Request.Context(), userID, projectID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, project)
}

// List retrieves paginated projects for the authenticated user.
// @Summary      List projects
// @Description  Retrieve paginated projects using keyset cursor or offset paging
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        cursor query string false "Opaque keyset pagination cursor"
// @Param        page query int false "Offset page number (default 1)"
// @Param        limit query int false "Number of items per page (default 10, max 100)"
// @Success      200 {object} service.PaginatedProjectsResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Router       /api/v1/projects [get]
func (h *ProjectHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var query ListProjectsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.projectService.ListProjects(c.Request.Context(), userID, service.PaginationParams{
		Cursor: query.Cursor,
		Page:   query.Page,
		Limit:  query.Limit,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Update updates an existing project with optimistic concurrency control.
// @Summary      Update project
// @Description  Update project details with optimistic concurrency control (version check)
// @Tags         projects
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Param        payload body UpdateProjectRequest true "Update payload with current version"
// @Success      200 {object} service.ProjectResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      409 {object} ProblemDetails
// @Router       /api/v1/projects/{id} [put]
// @Router       /api/v1/projects/{id} [patch]
func (h *ProjectHandler) Update(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	updated, err := h.projectService.UpdateProject(c.Request.Context(), userID, projectID, service.UpdateProjectInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		Version:     req.Version,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// Delete soft-deletes a project owned by the user.
// @Summary      Delete project
// @Description  Soft delete project owned by the authenticated user
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/projects/{id} [delete]
func (h *ProjectHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.projectService.DeleteProject(c.Request.Context(), userID, projectID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Project deleted successfully",
	})
}
