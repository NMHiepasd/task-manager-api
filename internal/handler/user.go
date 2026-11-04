package handler

import (
	"errors"
	"net/http"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/response"

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
// GET /api/v1/users/me
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error(
			"UNAUTHORIZED",
			"User context could not be determined",
		))
		return
	}

	profile, err := h.userService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, response.Error(
				"USER_NOT_FOUND",
				err.Error(),
			))
			return
		}

		c.JSON(http.StatusInternalServerError, response.Error(
			"INTERNAL_SERVER_ERROR",
			"Failed to retrieve user profile",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(profile))
}
