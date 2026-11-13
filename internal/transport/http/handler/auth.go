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
// @Summary      Register a new user
// @Description  Create a new user account and obtain access and refresh JWT tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Param        payload body RegisterRequest true "Registration payload"
// @Success      201 {object} service.TokenResponse
// @Failure      400 {object} ProblemDetails
// @Failure      409 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/auth/register [post]
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
// @Summary      Authenticate user
// @Description  Log in using email and password to receive JWT tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Param        payload body LoginRequest true "Login credentials"
// @Success      200 {object} service.TokenResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/auth/login [post]
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
// @Summary      Rotate refresh token
// @Description  Exchange a valid refresh token for a newly issued token pair (with reuse detection)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Param        payload body RefreshTokenRequest true "Refresh token payload"
// @Success      200 {object} service.TokenResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/auth/refresh [post]
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
// @Summary      Revoke session / Logout
// @Description  Revoke the given refresh token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Param        payload body LogoutRequest true "Logout payload"
// @Success      200 {object} map[string]string
// @Failure      400 {object} ProblemDetails
// @Failure      429 {object} ProblemDetails
// @Router       /api/v1/auth/logout [post]
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
