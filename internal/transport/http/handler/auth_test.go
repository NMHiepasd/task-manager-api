package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mockAuthService struct {
	registerFn     func(ctx context.Context, input service.RegisterInput) (*service.TokenResponse, error)
	loginFn        func(ctx context.Context, input service.LoginInput) (*service.TokenResponse, error)
	refreshTokenFn func(ctx context.Context, plainRefreshToken string) (*service.TokenResponse, error)
	logoutFn       func(ctx context.Context, plainRefreshToken string) error
}

func (m *mockAuthService) Register(ctx context.Context, input service.RegisterInput) (*service.TokenResponse, error) {
	if m.registerFn != nil {
		return m.registerFn(ctx, input)
	}
	return nil, nil
}

func (m *mockAuthService) Login(ctx context.Context, input service.LoginInput) (*service.TokenResponse, error) {
	if m.loginFn != nil {
		return m.loginFn(ctx, input)
	}
	return nil, nil
}

func (m *mockAuthService) RefreshToken(ctx context.Context, plainRefreshToken string) (*service.TokenResponse, error) {
	if m.refreshTokenFn != nil {
		return m.refreshTokenFn(ctx, plainRefreshToken)
	}
	return nil, nil
}

func (m *mockAuthService) Logout(ctx context.Context, plainRefreshToken string) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, plainRefreshToken)
	}
	return nil
}

func TestAuthHandler_Register(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Created_201", func(t *testing.T) {
		mock := &mockAuthService{
			registerFn: func(_ context.Context, input service.RegisterInput) (*service.TokenResponse, error) {
				return &service.TokenResponse{
					AccessToken:  "access_token",
					RefreshToken: "refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    900,
					User: service.UserResponse{
						ID:        uuid.New(),
						Email:     input.Email,
						FullName:  input.FullName,
						Role:      "user",
						CreatedAt: time.Now(),
					},
				}, nil
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/register", handler.Register)

		body := map[string]string{
			"email":     "test@example.com",
			"password":  "SecretPassword123!",
			"full_name": "Test User",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("BadRequest_400_ShortPassword", func(t *testing.T) {
		handler := NewAuthHandler(&mockAuthService{})
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/register", handler.Register)

		body := map[string]string{
			"email":     "test@example.com",
			"password":  "123", // Too short
			"full_name": "Test User",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})

	t.Run("Conflict_409_EmailExists", func(t *testing.T) {
		mock := &mockAuthService{
			registerFn: func(_ context.Context, _ service.RegisterInput) (*service.TokenResponse, error) {
				return nil, service.ErrEmailAlreadyExists
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/register", handler.Register)

		body := map[string]string{
			"email":     "existing@example.com",
			"password":  "SecretPassword123!",
			"full_name": "Test User",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusConflict, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestAuthHandler_Login(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockAuthService{
			loginFn: func(_ context.Context, _ service.LoginInput) (*service.TokenResponse, error) {
				return &service.TokenResponse{
					AccessToken:  "access_token",
					RefreshToken: "refresh_token",
					TokenType:    "Bearer",
				}, nil
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/login", handler.Login)

		body := map[string]string{
			"email":    "user@example.com",
			"password": "CorrectPassword123!",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Unauthorized_401", func(t *testing.T) {
		mock := &mockAuthService{
			loginFn: func(_ context.Context, _ service.LoginInput) (*service.TokenResponse, error) {
				return nil, service.ErrInvalidCredentials
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/login", handler.Login)

		body := map[string]string{
			"email":    "user@example.com",
			"password": "WrongPassword!",
		}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestAuthHandler_RefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("OK_200", func(t *testing.T) {
		mock := &mockAuthService{
			refreshTokenFn: func(_ context.Context, _ string) (*service.TokenResponse, error) {
				return &service.TokenResponse{
					AccessToken:  "new_access_token",
					RefreshToken: "new_refresh_token",
				}, nil
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/refresh", handler.RefreshToken)

		body := map[string]string{"refresh_token": "valid_token"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/refresh", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Unauthorized_401_ReuseDetected", func(t *testing.T) {
		mock := &mockAuthService{
			refreshTokenFn: func(_ context.Context, _ string) (*service.TokenResponse, error) {
				return nil, service.ErrTokenReuseDetected
			},
		}

		handler := NewAuthHandler(mock)
		router := gin.New()
		router.Use(middleware.ErrorHandler(nil))
		router.POST("/refresh", handler.RefreshToken)

		body := map[string]string{"refresh_token": "reused_token"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/refresh", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mock := &mockAuthService{
		logoutFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	handler := NewAuthHandler(mock)
	router := gin.New()
	router.Use(middleware.ErrorHandler(nil))
	router.POST("/logout", handler.Logout)

	body := map[string]string{"refresh_token": "some_token"}
	jsonBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/logout", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
