package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/middleware"
	"task-manager-api/internal/service"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mockUserService struct {
	getProfileFn func(ctx context.Context, userID uuid.UUID) (*service.UserResponse, error)
}

func (m *mockUserService) GetProfile(ctx context.Context, userID uuid.UUID) (*service.UserResponse, error) {
	if m.getProfileFn != nil {
		return m.getProfileFn(ctx, userID)
	}
	return nil, nil
}

func TestUserHandler_GetMe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockUserService{
			getProfileFn: func(_ context.Context, id uuid.UUID) (*service.UserResponse, error) {
				return &service.UserResponse{
					ID:        id,
					Email:     "user@example.com",
					FullName:  "Test User",
					Role:      "user",
					CreatedAt: time.Now(),
				}, nil
			},
		}

		handler := NewUserHandler(mock)
		router := gin.New()
		router.GET("/me", func(c *gin.Context) {
			// Simulate AuthMiddleware setting the user ID
			c.Set(middleware.AuthorizationUserIDKey, userID)
			handler.GetMe(c)
		})

		req, _ := http.NewRequest(http.MethodGet, "/me", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Unauthorized_401_MissingContext", func(t *testing.T) {
		handler := NewUserHandler(&mockUserService{})
		router := gin.New()
		router.GET("/me", handler.GetMe)

		req, _ := http.NewRequest(http.MethodGet, "/me", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("NotFound_404_UserDeleted", func(t *testing.T) {
		mock := &mockUserService{
			getProfileFn: func(_ context.Context, _ uuid.UUID) (*service.UserResponse, error) {
				return nil, service.ErrUserNotFound
			},
		}

		handler := NewUserHandler(mock)
		router := gin.New()
		router.GET("/me", func(c *gin.Context) {
			c.Set(middleware.AuthorizationUserIDKey, userID)
			handler.GetMe(c)
		})

		req, _ := http.NewRequest(http.MethodGet, "/me", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}
