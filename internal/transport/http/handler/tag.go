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
// @Summary      Create tag
// @Description  Create a tag for labeling tasks
// @Tags         tags
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        payload body CreateTagRequest true "Tag payload"
// @Success      201 {object} service.TagResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Router       /api/v1/tags [post]
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
// @Summary      List tags
// @Description  Retrieve all tags owned by the authenticated user
// @Tags         tags
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Success      200 {array} service.TagResponse
// @Failure      401 {object} ProblemDetails
// @Router       /api/v1/tags [get]
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
// @Summary      Delete tag
// @Description  Delete a tag by ID
// @Tags         tags
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Tag UUID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tags/{id} [delete]
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
