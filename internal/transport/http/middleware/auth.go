package middleware

import (
	"errors"
	"net/http"
	"strings"
	"task-manager-api/internal/domain"
	"task-manager-api/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// AuthorizationHeaderKey is the standard HTTP header for Bearer tokens.
	AuthorizationHeaderKey = "Authorization"
	// AuthorizationTypeBearer defines the expected token prefix.
	AuthorizationTypeBearer = "bearer"
	// AuthorizationPayloadKey is the context key for the token payload.
	AuthorizationPayloadKey = "authorization_payload"
	// AuthorizationUserIDKey is the context key for user UUID.
	AuthorizationUserIDKey = "auth_user_id"
)

// AuthMiddleware creates a Gin middleware that authenticates requests using JWT access tokens.
func AuthMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := GetRequestID(c)
		path := c.Request.URL.Path

		authHeader := c.GetHeader(AuthorizationHeaderKey)
		if len(authHeader) == 0 {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusUnauthorized, domain.NewProblem(
				http.StatusUnauthorized,
				"Authorization header is missing",
				"UNAUTHORIZED",
				path,
				reqID,
			))
			return
		}

		fields := strings.Fields(authHeader)
		if len(fields) < 2 || strings.ToLower(fields[0]) != AuthorizationTypeBearer {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusUnauthorized, domain.NewProblem(
				http.StatusUnauthorized,
				"Invalid authorization header format; expected 'Bearer <token>'",
				"UNAUTHORIZED",
				path,
				reqID,
			))
			return
		}

		accessToken := fields[1]
		payload, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			code := "UNAUTHORIZED"
			if errors.Is(err, token.ErrExpiredToken) {
				code = "TOKEN_EXPIRED"
			}
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusUnauthorized, domain.NewProblem(
				http.StatusUnauthorized,
				err.Error(),
				code,
				path,
				reqID,
			))
			return
		}

		c.Set(AuthorizationPayloadKey, payload)
		c.Set(AuthorizationUserIDKey, payload.UserID)
		c.Next()
	}
}

// GetAuthPayload extracts the authenticated UserClaims from context.
func GetAuthPayload(c *gin.Context) (*token.UserClaims, error) {
	val, exists := c.Get(AuthorizationPayloadKey)
	if !exists {
		return nil, domain.ErrUnauthorized
	}

	claims, ok := val.(*token.UserClaims)
	if !ok {
		return nil, domain.ErrUnauthorized
	}

	return claims, nil
}

// GetAuthUserID extracts the authenticated user's UUID from context.
func GetAuthUserID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(AuthorizationUserIDKey)
	if !exists {
		return uuid.Nil, domain.ErrUnauthorized
	}

	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil, domain.ErrUnauthorized
	}

	return userID, nil
}
