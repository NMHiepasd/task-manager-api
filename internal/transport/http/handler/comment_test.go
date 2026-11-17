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

type mockCommentService struct {
	createCommentFn func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input service.CreateCommentInput) (*service.CommentResponse, error)
	listCommentsFn  func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, params service.PaginationParams) (*service.PaginatedCommentsResponse, error)
	updateCommentFn func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID, input service.UpdateCommentInput) (*service.CommentResponse, error)
	deleteCommentFn func(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID) error
}

func (m *mockCommentService) CreateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input service.CreateCommentInput) (*service.CommentResponse, error) {
	if m.createCommentFn != nil {
		return m.createCommentFn(ctx, userID, taskID, input)
	}
	return nil, nil
}

func (m *mockCommentService) ListComments(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, params service.PaginationParams) (*service.PaginatedCommentsResponse, error) {
	if m.listCommentsFn != nil {
		return m.listCommentsFn(ctx, userID, taskID, params)
	}
	return nil, nil
}

func (m *mockCommentService) UpdateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID, input service.UpdateCommentInput) (*service.CommentResponse, error) {
	if m.updateCommentFn != nil {
		return m.updateCommentFn(ctx, userID, taskID, commentID, input)
	}
	return nil, nil
}

func (m *mockCommentService) DeleteComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID) error {
	if m.deleteCommentFn != nil {
		return m.deleteCommentFn(ctx, userID, taskID, commentID)
	}
	return nil
}

func setupTestCommentRouter(handler *CommentHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(nil))
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})

	router.POST("/tasks/:id/comments", handler.Create)
	router.GET("/tasks/:id/comments", handler.List)
	router.PATCH("/tasks/:id/comments/:commentId", handler.Update)
	router.DELETE("/tasks/:id/comments/:commentId", handler.Delete)

	return router
}

func TestCommentHandler_Create(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Created_201", func(t *testing.T) {
		mock := &mockCommentService{
			createCommentFn: func(_ context.Context, uID uuid.UUID, tID uuid.UUID, input service.CreateCommentInput) (*service.CommentResponse, error) {
				return &service.CommentResponse{
					ID:        uuid.New(),
					TaskID:    tID,
					UserID:    uID,
					Author:    "Test Author",
					Content:   input.Content,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		}

		handler := NewCommentHandler(mock)
		router := setupTestCommentRouter(handler, userID)

		body := map[string]string{"content": "Great progress!"}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/tasks/"+taskID.String()+"/comments", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("BadRequest_MissingContent", func(t *testing.T) {
		handler := NewCommentHandler(&mockCommentService{})
		router := setupTestCommentRouter(handler, userID)

		body := map[string]string{}
		jsonBytes, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/tasks/"+taskID.String()+"/comments", bytes.NewReader(jsonBytes))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestCommentHandler_List(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	mock := &mockCommentService{
		listCommentsFn: func(_ context.Context, _ uuid.UUID, tID uuid.UUID, _ service.PaginationParams) (*service.PaginatedCommentsResponse, error) {
			return &service.PaginatedCommentsResponse{
				Items: []service.CommentResponse{
					{ID: uuid.New(), TaskID: tID, Content: "Hello world", Author: "John"},
				},
				TotalItems: 1,
				Page:       1,
				Limit:      20,
			}, nil
		},
	}

	handler := NewCommentHandler(mock)
	router := setupTestCommentRouter(handler, userID)

	req, _ := http.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/comments", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCommentHandler_Update(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	mock := &mockCommentService{
		updateCommentFn: func(_ context.Context, uID uuid.UUID, tID uuid.UUID, cID uuid.UUID, input service.UpdateCommentInput) (*service.CommentResponse, error) {
			return &service.CommentResponse{
				ID:      cID,
				TaskID:  tID,
				UserID:  uID,
				Content: input.Content,
			}, nil
		},
	}

	handler := NewCommentHandler(mock)
	router := setupTestCommentRouter(handler, userID)

	body := map[string]string{"content": "Updated comment content"}
	jsonBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPatch, "/tasks/"+taskID.String()+"/comments/"+commentID.String(), bytes.NewReader(jsonBytes))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCommentHandler_Delete(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	mock := &mockCommentService{
		deleteCommentFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) error {
			return nil
		},
	}

	handler := NewCommentHandler(mock)
	router := setupTestCommentRouter(handler, userID)

	req, _ := http.NewRequest(http.MethodDelete, "/tasks/"+taskID.String()+"/comments/"+commentID.String(), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
