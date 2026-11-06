package handler

import (
	"errors"
	"net/http"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/response"
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
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid task input", err.Error()))
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
		if errors.Is(err, service.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, response.Error("PROJECT_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		if errors.Is(err, service.ErrInvalidTaskStatus) || errors.Is(err, service.ErrInvalidTaskPriority) {
			c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to create task"))
		return
	}

	c.JSON(http.StatusCreated, response.Success(task))
}

// GetByID retrieves task details by ID.
// GET /api/v1/tasks/:id
func (h *TaskHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid task UUID format"))
		return
	}

	task, err := h.taskService.GetTaskByID(c.Request.Context(), userID, taskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			c.JSON(http.StatusNotFound, response.Error("TASK_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to retrieve task"))
		return
	}

	c.JSON(http.StatusOK, response.Success(task))
}

// List retrieves paginated tasks based on criteria.
// GET /api/v1/tasks
func (h *TaskHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	var query ListTasksQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid query parameters", err.Error()))
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
		if errors.Is(err, service.ErrInvalidTaskStatus) || errors.Is(err, service.ErrInvalidTaskPriority) {
			c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to list tasks"))
		return
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// Update updates an existing task with optimistic locking check.
// PUT /api/v1/tasks/:id
func (h *TaskHandler) Update(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid task UUID format"))
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid update payload", err.Error()))
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
		if errors.Is(err, service.ErrTaskNotFound) {
			c.JSON(http.StatusNotFound, response.Error("TASK_NOT_FOUND", err.Error()))
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
		if errors.Is(err, service.ErrInvalidTaskStatus) || errors.Is(err, service.ErrInvalidTaskPriority) {
			c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to update task"))
		return
	}

	c.JSON(http.StatusOK, response.Success(updated))
}

// Delete soft-deletes a task.
// DELETE /api/v1/tasks/:id
func (h *TaskHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid task UUID format"))
		return
	}

	if err := h.taskService.DeleteTask(c.Request.Context(), userID, taskID); err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			c.JSON(http.StatusNotFound, response.Error("TASK_NOT_FOUND", err.Error()))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, response.Error("FORBIDDEN", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to delete task"))
		return
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"message": "Task deleted successfully",
	}))
}
