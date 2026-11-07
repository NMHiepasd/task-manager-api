package handler

import (
	"net/http"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateTaskRequest defines payload for creating a task.
type CreateTaskRequest struct {
	ProjectID   *uuid.UUID  `json:"project_id"`
	AssigneeID  *uuid.UUID  `json:"assignee_id"`
	Title       string      `json:"title" binding:"required,min=1,max=255"`
	Description string      `json:"description" binding:"max=5000"`
	Status      string      `json:"status" binding:"omitempty,oneof=todo in_progress completed cancelled"`
	Priority    string      `json:"priority" binding:"omitempty,oneof=low medium high urgent"`
	DueDate     *time.Time  `json:"due_date"`
	TagIDs      []uuid.UUID `json:"tag_ids"`
}

// UpdateTaskRequest defines payload for updating a task.
type UpdateTaskRequest struct {
	ProjectID   *uuid.UUID  `json:"project_id"`
	AssigneeID  *uuid.UUID  `json:"assignee_id"`
	Title       string      `json:"title" binding:"omitempty,min=1,max=255"`
	Description string      `json:"description" binding:"max=5000"`
	Status      string      `json:"status" binding:"omitempty,oneof=todo in_progress completed cancelled"`
	Priority    string      `json:"priority" binding:"omitempty,oneof=low medium high urgent"`
	DueDate     *time.Time  `json:"due_date"`
	Version     int32       `json:"version" binding:"required,min=1"`
	TagIDs      []uuid.UUID `json:"tag_ids"`
}

// SetTaskTagsRequest defines payload for updating task tags.
type SetTaskTagsRequest struct {
	TagIDs []uuid.UUID `json:"tag_ids"`
}

// ListTasksQuery defines query filters for listing tasks.
type ListTasksQuery struct {
	ProjectID *uuid.UUID `form:"project_id"`
	Status    *string    `form:"status" binding:"omitempty,oneof=todo in_progress completed cancelled"`
	Priority  *string    `form:"priority" binding:"omitempty,oneof=low medium high urgent"`
	TagID     *uuid.UUID `form:"tag_id"`
	Search    *string    `form:"q"`
	DueBefore *time.Time `form:"due_before" time_format:"2006-01-02T15:04:05Z07:00"`
	Cursor    string     `form:"cursor"`
	Page      int32      `form:"page,default=1" binding:"omitempty,min=1"`
	Limit     int32      `form:"limit,default=20" binding:"omitempty,min=1,max=100"`
}

// TaskHandler handles HTTP requests for tasks.
type TaskHandler struct {
	taskService service.TaskService
}

// NewTaskHandler constructs a new TaskHandler.
func NewTaskHandler(taskService service.TaskService) *TaskHandler {
	return &TaskHandler{taskService: taskService}
}

// Create creates a task.
// @Summary      Create task
// @Description  Create a new task with project assignment, tags, priority, and due date
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        payload body CreateTaskRequest true "Task payload"
// @Success      201 {object} service.TaskResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks [post]
func (h *TaskHandler) Create(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	task, err := h.taskService.CreateTask(c.Request.Context(), userID, service.CreateTaskInput{
		ProjectID:   req.ProjectID,
		AssigneeID:  req.AssigneeID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
		TagIDs:      req.TagIDs,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, task)
}

// GetByID retrieves task details by ID.
// @Summary      Get task by ID
// @Description  Retrieve task details and associated tags by ID (enforcing anti-IDOR 404)
// @Tags         tasks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Success      200 {object} service.TaskResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id} [get]
func (h *TaskHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	task, err := h.taskService.GetTaskByID(c.Request.Context(), userID, taskID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, task)
}

// List retrieves paginated tasks based on criteria.
// @Summary      List tasks
// @Description  Search and filter tasks with Keyset Pagination or Offset Paging, full-text search, and filtering
// @Tags         tasks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        project_id query string false "Filter by Project UUID"
// @Param        status query string false "Filter by status (todo, in_progress, completed, cancelled)"
// @Param        priority query string false "Filter by priority (low, medium, high, urgent)"
// @Param        tag_id query string false "Filter by Tag UUID"
// @Param        q query string false "Full-text search query across title and description"
// @Param        due_before query string false "Filter tasks due before RFC3339 timestamp"
// @Param        cursor query string false "Opaque keyset pagination cursor"
// @Param        page query int false "Offset page number (default 1)"
// @Param        limit query int false "Items per page (default 20, max 100)"
// @Success      200 {object} service.PaginatedTasksResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Router       /api/v1/tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var query ListTasksQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.taskService.ListTasks(c.Request.Context(), userID, service.ListTasksFilter{
		ProjectID: query.ProjectID,
		Status:    query.Status,
		Priority:  query.Priority,
		TagID:     query.TagID,
		Search:    query.Search,
		DueBefore: query.DueBefore,
		Cursor:    query.Cursor,
		Page:      query.Page,
		Limit:     query.Limit,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListProjectTasks retrieves all tasks for a specific project.
// @Summary      List tasks for a project
// @Description  Retrieve all tasks under a specific project (with anti-IDOR 404)
// @Tags         projects
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Project UUID"
// @Param        status query string false "Filter by status"
// @Param        priority query string false "Filter by priority"
// @Param        tag_id query string false "Filter by Tag UUID"
// @Param        q query string false "Full-text search query"
// @Param        due_before query string false "Due before RFC3339"
// @Param        cursor query string false "Keyset cursor"
// @Param        page query int false "Page number"
// @Param        limit query int false "Limit"
// @Success      200 {object} service.PaginatedTasksResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/projects/{id}/tasks [get]
func (h *TaskHandler) ListProjectTasks(c *gin.Context) {
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

	var query ListTasksQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.taskService.ListProjectTasks(c.Request.Context(), userID, projectID, service.ListTasksFilter{
		Status:    query.Status,
		Priority:  query.Priority,
		TagID:     query.TagID,
		Search:    query.Search,
		DueBefore: query.DueBefore,
		Cursor:    query.Cursor,
		Page:      query.Page,
		Limit:     query.Limit,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// SetTags replaces all tags for a task.
// @Summary      Replace task tags
// @Description  Atomic batch replacement of all tags assigned to a task in a single transaction
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        payload body SetTaskTagsRequest true "Array of Tag UUIDs"
// @Success      200 {array} service.TagResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/tags [put]
func (h *TaskHandler) SetTags(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req SetTaskTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	tags, err := h.taskService.SetTaskTags(c.Request.Context(), userID, taskID, req.TagIDs)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, tags)
}

// Update updates an existing task with optimistic locking check.
// @Summary      Update task
// @Description  Update task details with optimistic concurrency control (version check)
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        payload body UpdateTaskRequest true "Update payload with current version"
// @Success      200 {object} service.TaskResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      409 {object} ProblemDetails
// @Router       /api/v1/tasks/{id} [put]
// @Router       /api/v1/tasks/{id} [patch]
func (h *TaskHandler) Update(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	updated, err := h.taskService.UpdateTask(c.Request.Context(), userID, taskID, service.UpdateTaskInput{
		ProjectID:   req.ProjectID,
		AssigneeID:  req.AssigneeID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
		Version:     req.Version,
		TagIDs:      req.TagIDs,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// Delete soft-deletes a task.
// @Summary      Delete task
// @Description  Soft delete task owned by the authenticated user
// @Tags         tasks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id} [delete]
func (h *TaskHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.taskService.DeleteTask(c.Request.Context(), userID, taskID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Task deleted successfully",
	})
}
