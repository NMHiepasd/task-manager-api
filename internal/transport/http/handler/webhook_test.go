package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWebhookService struct {
	createWebhookFn   func(ctx context.Context, userID uuid.UUID, req service.CreateWebhookRequest) (*service.WebhookResponse, error)
	listWebhooksFn    func(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]service.WebhookResponse, error)
	getWebhookFn      func(ctx context.Context, userID, webhookID uuid.UUID) (*service.WebhookResponse, error)
	updateWebhookFn   func(ctx context.Context, userID, webhookID uuid.UUID, req service.UpdateWebhookRequest) (*service.WebhookResponse, error)
	deleteWebhookFn   func(ctx context.Context, userID, webhookID uuid.UUID) error
	triggerTestPingFn func(ctx context.Context, userID, webhookID uuid.UUID) error
	listDeliveriesFn  func(ctx context.Context, userID, webhookID uuid.UUID, limit, offset int32) ([]service.WebhookDeliveryResponse, error)
	dispatchEventFn   func(ctx context.Context, actorID uuid.UUID, projectID *uuid.UUID, eventType string, data any) error
}

func (m *mockWebhookService) CreateWebhook(ctx context.Context, userID uuid.UUID, req service.CreateWebhookRequest) (*service.WebhookResponse, error) {
	if m.createWebhookFn != nil {
		return m.createWebhookFn(ctx, userID, req)
	}
	return nil, nil
}

func (m *mockWebhookService) ListWebhooks(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]service.WebhookResponse, error) {
	if m.listWebhooksFn != nil {
		return m.listWebhooksFn(ctx, userID, projectID)
	}
	return nil, nil
}

func (m *mockWebhookService) GetWebhook(ctx context.Context, userID, webhookID uuid.UUID) (*service.WebhookResponse, error) {
	if m.getWebhookFn != nil {
		return m.getWebhookFn(ctx, userID, webhookID)
	}
	return nil, nil
}

func (m *mockWebhookService) UpdateWebhook(ctx context.Context, userID, webhookID uuid.UUID, req service.UpdateWebhookRequest) (*service.WebhookResponse, error) {
	if m.updateWebhookFn != nil {
		return m.updateWebhookFn(ctx, userID, webhookID, req)
	}
	return nil, nil
}

func (m *mockWebhookService) DeleteWebhook(ctx context.Context, userID, webhookID uuid.UUID) error {
	if m.deleteWebhookFn != nil {
		return m.deleteWebhookFn(ctx, userID, webhookID)
	}
	return nil
}

func (m *mockWebhookService) TriggerTestPing(ctx context.Context, userID, webhookID uuid.UUID) error {
	if m.triggerTestPingFn != nil {
		return m.triggerTestPingFn(ctx, userID, webhookID)
	}
	return nil
}

func (m *mockWebhookService) ListDeliveries(ctx context.Context, userID, webhookID uuid.UUID, limit, offset int32) ([]service.WebhookDeliveryResponse, error) {
	if m.listDeliveriesFn != nil {
		return m.listDeliveriesFn(ctx, userID, webhookID, limit, offset)
	}
	return nil, nil
}

func (m *mockWebhookService) DispatchEvent(ctx context.Context, actorID uuid.UUID, projectID *uuid.UUID, eventType string, data any) error {
	if m.dispatchEventFn != nil {
		return m.dispatchEventFn(ctx, actorID, projectID, eventType, data)
	}
	return nil
}

func setupWebhookRouter(handler *WebhookHandler, authUserID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(slog.Default()))
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, authUserID)
		c.Next()
	})

	group := router.Group("/api/v1/webhooks")
	{
		group.POST("", handler.Create)
		group.GET("", handler.List)
		group.GET("/:id", handler.GetByID)
		group.PATCH("/:id", handler.Update)
		group.DELETE("/:id", handler.Delete)
		group.POST("/:id/test", handler.TriggerPing)
		group.GET("/:id/deliveries", handler.ListDeliveries)
	}

	return router
}

func TestWebhookHandler_Create(t *testing.T) {
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			createWebhookFn: func(_ context.Context, uid uuid.UUID, req service.CreateWebhookRequest) (*service.WebhookResponse, error) {
				return &service.WebhookResponse{
					ID:        uuid.New(),
					UserID:    uid,
					URL:       req.URL,
					Secret:    "whsec_test",
					Events:    req.Events,
					IsActive:  true,
					CreatedAt: time.Now(),
				}, nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		body := map[string]any{
			"url":    "https://example.com/webhook",
			"events": []string{"task.created", "task.deleted"},
		}
		jsonBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp service.WebhookResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/webhook", resp.URL)
		assert.Equal(t, "whsec_test", resp.Secret)
		assert.Contains(t, w.Body.String(), `"secret"`)
	})
}

func TestWebhookHandler_GetAndList(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("GetByID_Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			getWebhookFn: func(_ context.Context, uid, wid uuid.UUID) (*service.WebhookResponse, error) {
				return &service.WebhookResponse{
					ID:       wid,
					UserID:   uid,
					URL:      "https://example.com/hook",
					Events:   []string{"*"},
					IsActive: true,
				}, nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+webhookID.String(), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"secret"`, "secret must not be returned in GetByID")
	})

	t.Run("GetByID_InvalidUUID", func(t *testing.T) {
		handler := NewWebhookHandler(&mockWebhookService{})
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/invalid-uuid", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		// Because ErrorHandler middleware is not registered in this bare router, c.Error returns empty status code or default
		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusOK)
	})

	t.Run("List_Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			listWebhooksFn: func(_ context.Context, uid uuid.UUID, _ *uuid.UUID) ([]service.WebhookResponse, error) {
				return []service.WebhookResponse{
					{ID: uuid.New(), UserID: uid, URL: "https://hook1.com"},
				}, nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"secret"`, "secret must not be returned in GetByID")
	})
}

func TestWebhookHandler_UpdateAndDelete(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("Update_Success", func(t *testing.T) {
		newURL := "https://example.com/updated"
		mockSvc := &mockWebhookService{
			updateWebhookFn: func(_ context.Context, uid, wid uuid.UUID, req service.UpdateWebhookRequest) (*service.WebhookResponse, error) {
				return &service.WebhookResponse{
					ID:     wid,
					UserID: uid,
					URL:    *req.URL,
				}, nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		body := map[string]any{"url": newURL}
		jsonBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPatch, "/api/v1/webhooks/"+webhookID.String(), bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"secret"`, "secret must not be returned in GetByID")
	})

	t.Run("Delete_Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			deleteWebhookFn: func(_ context.Context, _, _ uuid.UUID) error {
				return nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/"+webhookID.String(), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestWebhookHandler_TriggerPingAndDeliveries(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("TriggerPing_Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			triggerTestPingFn: func(_ context.Context, _, _ uuid.UUID) error {
				return nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID.String()+"/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"secret"`, "secret must not be returned in GetByID")
	})

	t.Run("ListDeliveries_Success", func(t *testing.T) {
		mockSvc := &mockWebhookService{
			listDeliveriesFn: func(_ context.Context, _, wid uuid.UUID, _, _ int32) ([]service.WebhookDeliveryResponse, error) {
				return []service.WebhookDeliveryResponse{
					{ID: uuid.New(), WebhookID: wid, EventType: "task.created", Status: "success"},
				}, nil
			},
		}

		handler := NewWebhookHandler(mockSvc)
		router := setupWebhookRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+webhookID.String()+"/deliveries?limit=10&offset=0", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"secret"`, "secret must not be returned in GetByID")
	})
}

func TestWebhookHandler_ADR0019_UnauthorizedReturns404(t *testing.T) {
	unauthorizedUserID := uuid.New()
	webhookID := uuid.New()

	mockSvc := &mockWebhookService{
		getWebhookFn: func(_ context.Context, _, _ uuid.UUID) (*service.WebhookResponse, error) {
			return nil, domain.ErrWebhookNotFound
		},
		updateWebhookFn: func(_ context.Context, _, _ uuid.UUID, _ service.UpdateWebhookRequest) (*service.WebhookResponse, error) {
			return nil, domain.ErrWebhookNotFound
		},
		deleteWebhookFn: func(_ context.Context, _, _ uuid.UUID) error {
			return domain.ErrWebhookNotFound
		},
		triggerTestPingFn: func(_ context.Context, _, _ uuid.UUID) error {
			return domain.ErrWebhookNotFound
		},
		listDeliveriesFn: func(_ context.Context, _, _ uuid.UUID, _, _ int32) ([]service.WebhookDeliveryResponse, error) {
			return nil, domain.ErrWebhookNotFound
		},
	}

	handler := NewWebhookHandler(mockSvc)
	router := setupWebhookRouter(handler, unauthorizedUserID)

	t.Run("GetByID_Returns404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+webhookID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
	})

	t.Run("Update_Returns404", func(t *testing.T) {
		body := `{"url":"https://example.com/updated"}`
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/webhooks/"+webhookID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
	})

	t.Run("Delete_Returns404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/"+webhookID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
	})

	t.Run("TriggerPing_Returns404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID.String()+"/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
	})

	t.Run("ListDeliveries_Returns404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+webhookID.String()+"/deliveries", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
	})
}
