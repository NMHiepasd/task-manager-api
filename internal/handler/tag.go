package handler

import (
	"net/http"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/response"

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
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error("BAD_REQUEST", "Invalid tag input", err.Error()))
		return
	}

	tag, err := h.tagService.CreateTag(c.Request.Context(), userID, service.CreateTagInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to create tag"))
		return
	}

	c.JSON(http.StatusCreated, response.Success(tag))
}

// List retrieves all tags for the user.
// GET /api/v1/tags
func (h *TagHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	tags, err := h.tagService.ListTags(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to list tags"))
		return
	}

	c.JSON(http.StatusOK, response.Success(tags))
}

// Delete deletes a tag owned by the user.
// DELETE /api/v1/tags/:id
func (h *TagHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error("UNAUTHORIZED", "User context not found"))
		return
	}

	tagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error("INVALID_UUID", "Invalid tag UUID format"))
		return
	}

	if err := h.tagService.DeleteTag(c.Request.Context(), userID, tagID); err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("INTERNAL_SERVER_ERROR", "Failed to delete tag"))
		return
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"message": "Tag deleted successfully",
	}))
}
