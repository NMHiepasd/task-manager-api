package handler

import (
	"net/http"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
)

// UserHandler handles HTTP requests for user profile operations.
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler constructs a new UserHandler.
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// GetMe retrieves the currently authenticated user's profile.
// @Summary      Get current user profile
// @Description  Retrieve profile details for the authenticated user
// @Tags         users
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Success      200 {object} service.UserResponse
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/users/me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	profile, err := h.userService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, profile)
}
