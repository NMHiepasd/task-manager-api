package handler

import (
	"net/http"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateTagRequest defines payload for creating a tag.
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required,min=1,max=50"`
	Color string `json:"color" binding:"omitempty,max=20"`
}

// TagHandler handles HTTP requests for tags.
type TagHandler struct {
	tagService service.TagService
}

// NewTagHandler constructs a new TagHandler.
func NewTagHandler(tagService service.TagService) *TagHandler {
	return &TagHandler{tagService: tagService}
}

// Create creates or updates a tag.
// POST /api/v1/tags
func (h *TagHandler) Create(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	tag, err := h.tagService.CreateTag(c.Request.Context(), userID, service.CreateTagInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, tag)
}

// List retrieves all tags for the user.
// GET /api/v1/tags
func (h *TagHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	tags, err := h.tagService.ListTags(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, tags)
}

// Delete deletes a tag owned by the user.
// DELETE /api/v1/tags/:id
func (h *TagHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	tagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.tagService.DeleteTag(c.Request.Context(), userID, tagID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tag deleted successfully",
	})
}
