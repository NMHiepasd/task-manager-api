package handler

import (
	"net/http"
	"strconv"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// WebhookHandler handles HTTP endpoints for managing webhook subscriptions and delivery audits.
type WebhookHandler struct {
	webhookService service.WebhookService
}

// NewWebhookHandler constructs a new WebhookHandler instance.
func NewWebhookHandler(webhookService service.WebhookService) *WebhookHandler {
	return &WebhookHandler{
		webhookService: webhookService,
	}
}

// Create registers a new webhook subscription.
// @Summary      Register webhook
// @Description  Creates a new webhook subscription for real-time system events (task, comment, attachment). If secret is omitted, a cryptographically secure secret (>=24 bytes) is generated. Custom secrets must be >=32 characters. Full secret is returned only once in this response.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        payload body service.CreateWebhookRequest true "Webhook subscription configuration"
// @Success      201 {object} service.WebhookResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks [post]
func (h *WebhookHandler) Create(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var req service.CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid request payload",
			Params:  []domain.InvalidParam{{Name: "body", Reason: err.Error()}},
		})
		return
	}

	webhookResp, err := h.webhookService.CreateWebhook(c.Request.Context(), userID, req)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, webhookResp)
}

// List returns all webhook subscriptions belonging to the caller or scoped to a project.
// @Summary      List webhooks
// @Description  Retrieves all registered webhook subscriptions for the authenticated user.
// @Tags         webhooks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        project_id query string false "Filter by project UUID"
// @Success      200 {array} service.WebhookResponse
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks [get]
func (h *WebhookHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var projectID *uuid.UUID
	if pIDStr := c.Query("project_id"); pIDStr != "" {
		parsed, err := uuid.Parse(pIDStr)
		if err != nil {
			_ = c.Error(domain.ValidationErrors{
				Message: "Invalid UUID format in query parameter",
				Params:  []domain.InvalidParam{{Name: "project_id", Reason: "must be a valid UUID"}},
			})
			return
		}
		projectID = &parsed
	}

	webhooks, err := h.webhookService.ListWebhooks(c.Request.Context(), userID, projectID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, webhooks)
}

// GetByID retrieves a single webhook subscription by its UUID.
// @Summary      Get webhook details
// @Description  Retrieves configuration details of a webhook subscription.
// @Tags         webhooks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Webhook UUID"
// @Success      200 {object} service.WebhookResponse
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks/{id} [get]
func (h *WebhookHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	wh, err := h.webhookService.GetWebhook(c.Request.Context(), userID, webhookID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, wh)
}

// Update modifies configuration fields of a webhook subscription.
// @Summary      Update webhook
// @Description  Updates target URL, subscribed events, or active status of a webhook.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Webhook UUID"
// @Param        payload body service.UpdateWebhookRequest true "Updated webhook settings"
// @Success      200 {object} service.WebhookResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks/{id} [patch]
func (h *WebhookHandler) Update(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	var req service.UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid request payload",
			Params:  []domain.InvalidParam{{Name: "body", Reason: err.Error()}},
		})
		return
	}

	updated, err := h.webhookService.UpdateWebhook(c.Request.Context(), userID, webhookID, req)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// Delete removes a webhook subscription.
// @Summary      Delete webhook
// @Description  Permanently removes a webhook subscription.
// @Tags         webhooks
// @Security     BearerAuth
// @Param        id path string true "Webhook UUID"
// @Success      204 "No Content"
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks/{id} [delete]
func (h *WebhookHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.webhookService.DeleteWebhook(c.Request.Context(), userID, webhookID); err != nil {
		_ = c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}

// TriggerPing sends a test ping event to verify webhook delivery.
// @Summary      Test webhook ping
// @Description  Triggers a test ping event to the subscribed webhook endpoint to verify network and HMAC connectivity.
// @Tags         webhooks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Webhook UUID"
// @Success      200 {object} map[string]string
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks/{id}/test [post]
func (h *WebhookHandler) TriggerPing(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.webhookService.TriggerTestPing(c.Request.Context(), userID, webhookID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Webhook ping event enqueued for asynchronous delivery",
	})
}

// ListDeliveries retrieves recent delivery attempts and execution logs for a webhook.
// @Summary      List webhook deliveries
// @Description  Returns recent delivery history, HTTP response codes, and execution duration for audit and troubleshooting.
// @Tags         webhooks
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Webhook UUID"
// @Param        limit query int false "Number of records to retrieve (default 20, max 100)"
// @Param        offset query int false "Number of records to skip (default 0)"
// @Success      200 {array} service.WebhookDeliveryResponse
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/webhooks/{id}/deliveries [get]
func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	limit := int32(20)
	if limitStr := c.Query("limit"); limitStr != "" {
		if val, err := strconv.ParseInt(limitStr, 10, 32); err == nil && val > 0 && val <= 100 {
			limit = int32(val)
		}
	}

	offset := int32(0)
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if val, err := strconv.ParseInt(offsetStr, 10, 32); err == nil && val >= 0 {
			offset = int32(val)
		}
	}

	deliveries, err := h.webhookService.ListDeliveries(c.Request.Context(), userID, webhookID, limit, offset)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, deliveries)
}
