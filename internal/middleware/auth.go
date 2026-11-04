package middleware

import (
	"errors"
	"net/http"
	"strings"
	"task-manager-api/pkg/response"
	"task-manager-api/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// AuthorizationHeaderKey is the standard HTTP header key for bearer token.
	AuthorizationHeaderKey = "Authorization"
	// AuthorizationTypeBearer defines the expected token prefix.
	AuthorizationTypeBearer = "bearer"
	// AuthorizationPayloadKey is the gin context key for user claims payload.
	AuthorizationPayloadKey = "authorization_payload"
	// AuthorizationUserIDKey is the gin context key specifically for user UUID.
	AuthorizationUserIDKey = "auth_user_id"
)

var (
	// ErrNoAuthHeader indicates missing Authorization header.
	ErrNoAuthHeader = errors.New("authorization header is missing")
	// ErrInvalidAuthHeader indicates malformed Authorization header.
	ErrInvalidAuthHeader = errors.New("invalid authorization header format, expected 'Bearer <token>'")
	// ErrUnauthorized indicates missing or invalid authentication context.
	ErrUnauthorized = errors.New("unauthorized request")
)

// AuthMiddleware creates a Gin middleware that authenticates requests using JWT access tokens.
func AuthMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeaderKey)
		if len(authHeader) == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(
				"UNAUTHORIZED",
				ErrNoAuthHeader.Error(),
			))
			return
		}

		fields := strings.Fields(authHeader)
		if len(fields) < 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(
				"UNAUTHORIZED",
				ErrInvalidAuthHeader.Error(),
			))
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != AuthorizationTypeBearer {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(
				"UNAUTHORIZED",
				ErrInvalidAuthHeader.Error(),
			))
			return
		}

		accessToken := fields[1]
		payload, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(
				"UNAUTHORIZED",
				err.Error(),
			))
			return
		}

		c.Set(AuthorizationPayloadKey, payload)
		c.Set(AuthorizationUserIDKey, payload.UserID)
		c.Next()
	}
}

// GetAuthPayload extracts the authenticated UserClaims from gin context.
func GetAuthPayload(c *gin.Context) (*token.UserClaims, error) {
	val, exists := c.Get(AuthorizationPayloadKey)
	if !exists {
		return nil, ErrUnauthorized
	}

	claims, ok := val.(*token.UserClaims)
	if !ok {
		return nil, ErrUnauthorized
	}

	return claims, nil
}

// GetAuthUserID extracts the authenticated user's UUID from gin context.
func GetAuthUserID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(AuthorizationUserIDKey)
	if !exists {
		return uuid.Nil, ErrUnauthorized
	}

	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrUnauthorized
	}

	return userID, nil
}
