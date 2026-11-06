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

type mockTagService struct {
	createTagFn func(ctx context.Context, userID uuid.UUID, input service.CreateTagInput) (*service.TagResponse, error)
	listTagsFn  func(ctx context.Context, userID uuid.UUID) ([]service.TagResponse, error)
	deleteTagFn func(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error
}

func (m *mockTagService) CreateTag(ctx context.Context, userID uuid.UUID, input service.CreateTagInput) (*service.TagResponse, error) {
	if m.createTagFn != nil {
		return m.createTagFn(ctx, userID, input)
	}
	return nil, nil
}

func (m *mockTagService) ListTags(ctx context.Context, userID uuid.UUID) ([]service.TagResponse, error) {
	if m.listTagsFn != nil {
		return m.listTagsFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockTagService) DeleteTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	if m.deleteTagFn != nil {
		return m.deleteTagFn(ctx, userID, tagID)
	}
	return nil
}

func setupTestTagRouter(h *TagHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})
	r.POST("/tags", h.Create)
	r.GET("/tags", h.List)
	r.DELETE("/tags/:id", h.Delete)
	return r
}

func TestTagHandler_Create(t *testing.T) {
	userID := uuid.New()
	tagID := uuid.New()

	mock := &mockTagService{
		createTagFn: func(_ context.Context, uID uuid.UUID, input service.CreateTagInput) (*service.TagResponse, error) {
			return &service.TagResponse{
				ID:        tagID,
				UserID:    uID,
				Name:      input.Name,
				Color:     input.Color,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	h := NewTagHandler(mock)
	router := setupTestTagRouter(h, userID)

	body := map[string]string{"name": "Frontend", "color": "#3b82f6"}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/tags", bytes.NewReader(b))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestTagHandler_List(t *testing.T) {
	userID := uuid.New()

	mock := &mockTagService{
		listTagsFn: func(_ context.Context, _ uuid.UUID) ([]service.TagResponse, error) {
			return []service.TagResponse{
				{ID: uuid.New(), Name: "Dev"},
			}, nil
		},
	}

	h := NewTagHandler(mock)
	router := setupTestTagRouter(h, userID)

	req, _ := http.NewRequest(http.MethodGet, "/tags", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
