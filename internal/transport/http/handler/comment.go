package handler

import (
	"net/http"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateCommentRequest defines input for adding a comment to a task.
type CreateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

// UpdateCommentRequest defines input for updating a task comment.
type UpdateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

// ListCommentsQuery specifies query parameters for listing comments.
type ListCommentsQuery struct {
	Cursor string `form:"cursor"`
	Page   int32  `form:"page,default=1" binding:"omitempty,min=1"`
	Limit  int32  `form:"limit,default=20" binding:"omitempty,min=1,max=100"`
}

// CommentHandler handles HTTP requests for task comments.
type CommentHandler struct {
	commentService service.CommentService
}

// NewCommentHandler constructs a new CommentHandler.
func NewCommentHandler(commentService service.CommentService) *CommentHandler {
	return &CommentHandler{commentService: commentService}
}

// Create adds a new comment to a task.
// @Summary      Create task comment
// @Description  Add a comment to an existing task. Caller must have access to the task.
// @Tags         comments
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        payload body CreateCommentRequest true "Comment payload"
// @Success      201 {object} service.CommentResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/comments [post]
func (h *CommentHandler) Create(c *gin.Context) {
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

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	comment, err := h.commentService.CreateComment(c.Request.Context(), userID, taskID, service.CreateCommentInput{
		Content: req.Content,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, comment)
}

// List retrieves paginated comments for a task.
// @Summary      List task comments
// @Description  List comments on a task with keyset or page offset pagination. Caller must have access to the task.
// @Tags         comments
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        cursor query string false "Keyset cursor token"
// @Param        page query int false "Page number (offset-based)" default(1)
// @Param        limit query int false "Items per page" default(20)
// @Success      200 {object} service.PaginatedCommentsResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/comments [get]
func (h *CommentHandler) List(c *gin.Context) {
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

	var query ListCommentsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		_ = c.Error(err)
		return
	}

	comments, err := h.commentService.ListComments(c.Request.Context(), userID, taskID, service.PaginationParams{
		Cursor: query.Cursor,
		Page:   query.Page,
		Limit:  query.Limit,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, comments)
}

// Update edits an existing task comment.
// @Summary      Update task comment
// @Description  Update the content of a task comment. Only the author can update their comment.
// @Tags         comments
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        commentId path string true "Comment UUID"
// @Param        payload body UpdateCommentRequest true "Comment update payload"
// @Success      200 {object} service.CommentResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/comments/{commentId} [patch]
func (h *CommentHandler) Update(c *gin.Context) {
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

	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "commentId", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	updated, err := h.commentService.UpdateComment(c.Request.Context(), userID, taskID, commentID, service.UpdateCommentInput{
		Content: req.Content,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// Delete removes a task comment.
// @Summary      Delete task comment
// @Description  Delete a task comment. Can be performed by the author, task creator, or project owner.
// @Tags         comments
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        commentId path string true "Comment UUID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/comments/{commentId} [delete]
func (h *CommentHandler) Delete(c *gin.Context) {
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

	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "commentId", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.commentService.DeleteComment(c.Request.Context(), userID, taskID, commentID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Comment deleted successfully",
	})
}
