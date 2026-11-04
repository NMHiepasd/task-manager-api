package handler

import (
	"errors"
	"net/http"
	"task-manager-api/internal/service"
	"task-manager-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// RegisterRequest defines the input payload for user registration.
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	FullName string `json:"full_name" binding:"required,min=2,max=100"`
}

// LoginRequest defines credentials for authentication.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest holds the refresh token for rotation.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest holds the refresh token to revoke.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// AuthHandler handles HTTP requests for user authentication.
type AuthHandler struct {
	authService service.AuthService
}

// NewAuthHandler constructs a new AuthHandler.
func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register creates a new user and issues access/refresh tokens.
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			"BAD_REQUEST",
			"Invalid registration input",
			err.Error(),
		))
		return
	}

	result, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
	})
	if err != nil {
		if errors.Is(err, service.ErrEmailAlreadyExists) {
			c.JSON(http.StatusConflict, response.Error(
				"EMAIL_EXISTS",
				err.Error(),
			))
			return
		}

		if errors.Is(err, service.ErrPasswordTooLong) {
			c.JSON(http.StatusBadRequest, response.Error(
				"BAD_REQUEST",
				err.Error(),
			))
			return
		}

		c.JSON(http.StatusInternalServerError, response.Error(
			"INTERNAL_SERVER_ERROR",
			"Failed to complete user registration",
		))
		return
	}

	c.JSON(http.StatusCreated, response.Success(result))
}

// Login authenticates a user and returns an access/refresh token pair.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			"BAD_REQUEST",
			"Invalid login input",
			err.Error(),
		))
		return
	}

	result, err := h.authService.Login(c.Request.Context(), service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, response.Error(
				"UNAUTHORIZED",
				err.Error(),
			))
			return
		}

		c.JSON(http.StatusInternalServerError, response.Error(
			"INTERNAL_SERVER_ERROR",
			"Failed to authenticate user",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// RefreshToken rotates the current refresh token and issues a new pair.
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			"BAD_REQUEST",
			"Refresh token is required",
			err.Error(),
		))
		return
	}

	result, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrTokenReuseDetected) {
			c.JSON(http.StatusUnauthorized, response.Error(
				"TOKEN_REUSE_DETECTED",
				err.Error(),
			))
			return
		}

		if errors.Is(err, service.ErrTokenExpired) {
			c.JSON(http.StatusUnauthorized, response.Error(
				"TOKEN_EXPIRED",
				err.Error(),
			))
			return
		}

		if errors.Is(err, service.ErrInvalidToken) || errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, response.Error(
				"INVALID_TOKEN",
				"Refresh token is invalid or user was removed",
			))
			return
		}

		c.JSON(http.StatusInternalServerError, response.Error(
			"INTERNAL_SERVER_ERROR",
			"Failed to refresh session token",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// Logout revokes the given refresh token.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			"BAD_REQUEST",
			"Refresh token is required for logout",
			err.Error(),
		))
		return
	}

	if err := h.authService.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(
			"INTERNAL_SERVER_ERROR",
			"Failed to revoke session token",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"message": "Successfully logged out",
	}))
}
