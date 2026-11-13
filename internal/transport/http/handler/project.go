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

// AddProjectMemberRequest defines input payload for adding a project member.
type AddProjectMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required,oneof=editor viewer"`
}

// UpdateProjectMemberRoleRequest defines input payload for updating a member's role.
type UpdateProjectMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=editor viewer"`
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

// AddMember adds a collaborator to the project by email.
// @Summary      Add project member
// @Description  Add a collaborator to the project by email with role editor or viewer. Only project owner can perform this.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Param        payload body AddProjectMemberRequest true "Member payload"
// @Success      201 {object} service.ProjectMemberResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      409 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/projects/{id}/members [post]
func (h *ProjectHandler) AddMember(c *gin.Context) {
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

	var req AddProjectMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	member, err := h.projectService.AddMember(c.Request.Context(), userID, projectID, service.AddProjectMemberInput{
		Email: req.Email,
		Role:  req.Role,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, member)
}

// ListMembers lists all collaborators in a project.
// @Summary      List project members
// @Description  List all members of a project. Caller must be a project member.
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Success      200 {array} service.ProjectMemberResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/projects/{id}/members [get]
func (h *ProjectHandler) ListMembers(c *gin.Context) {
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

	members, err := h.projectService.ListMembers(c.Request.Context(), userID, projectID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, members)
}

// UpdateMemberRole updates a member's role in the project.
// @Summary      Update member role
// @Description  Update a collaborator's role in the project (editor or viewer). Only project owner can perform this.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Param        userId path string true "User UUID of collaborator"
// @Param        payload body UpdateProjectMemberRoleRequest true "Role payload"
// @Success      200 {object} service.ProjectMemberResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/projects/{id}/members/{userId} [patch]
func (h *ProjectHandler) UpdateMemberRole(c *gin.Context) {
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

	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "userId", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req UpdateProjectMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	updated, err := h.projectService.UpdateMemberRole(c.Request.Context(), userID, projectID, targetUserID, service.UpdateProjectMemberRoleInput{
		Role: req.Role,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// RemoveMember removes a collaborator from the project or allows a member to leave.
// @Summary      Remove project member
// @Description  Remove a collaborator from the project (owner can remove members, members can remove themselves).
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Param        userId path string true "User UUID of collaborator"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/projects/{id}/members/{userId} [delete]
func (h *ProjectHandler) RemoveMember(c *gin.Context) {
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

	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "userId", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.projectService.RemoveMember(c.Request.Context(), userID, projectID, targetUserID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Project member removed successfully",
	})
}
