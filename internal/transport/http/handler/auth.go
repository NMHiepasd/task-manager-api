package handler

import (
	"net/http"
	"task-manager-api/internal/service"

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
		_ = c.Error(err)
		return
	}

	result, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, result)
}

// Login authenticates a user and returns an access/refresh token pair.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.authService.Login(c.Request.Context(), service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// RefreshToken rotates the current refresh token and issues a new pair.
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Logout revokes the given refresh token.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	if err := h.authService.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully logged out",
	})
}
