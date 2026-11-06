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

// ListTasksQuery defines query filters for listing tasks.
type ListTasksQuery struct {
	ProjectID *uuid.UUID `form:"project_id"`
	Status    *string    `form:"status" binding:"omitempty,oneof=todo in_progress completed cancelled"`
	Priority  *string    `form:"priority" binding:"omitempty,oneof=low medium high urgent"`
	Page      int32      `form:"page,default=1" binding:"omitempty,min=1"`
	Limit     int32      `form:"limit,default=10" binding:"omitempty,min=1,max=100"`
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
// POST /api/v1/tasks
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
// GET /api/v1/tasks/:id
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
// GET /api/v1/tasks
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
		Page:      query.Page,
		Limit:     query.Limit,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Update updates an existing task with optimistic locking check.
// PUT /api/v1/tasks/:id
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
// DELETE /api/v1/tasks/:id
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
