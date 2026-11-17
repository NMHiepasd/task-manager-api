package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/worker"
	"task-manager-api/pkg/webhook"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WebhookResponse represents public metadata for a webhook subscription.
type WebhookResponse struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
	URL       string     `json:"url"`
	Secret    string     `json:"secret,omitempty"` // Signing secret (returned only once upon creation; omitted in get/list/update)
	Events    []string   `json:"events"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// WebhookDeliveryResponse represents metadata and execution details of a webhook delivery attempt.
type WebhookDeliveryResponse struct {
	ID                  uuid.UUID `json:"id"`
	WebhookID           uuid.UUID `json:"webhook_id"`
	EventType           string    `json:"event_type"`
	Payload             string    `json:"payload"`
	ResponseStatus      *int32    `json:"response_status,omitempty"`
	ResponseBody        *string   `json:"response_body,omitempty"`
	ExecutionDurationMs *int32    `json:"execution_duration_ms,omitempty"`
	Status              string    `json:"status"`
	DeliveredAt         time.Time `json:"delivered_at"`
}

// CreateWebhookRequest defines the payload for creating a new webhook subscription.
type CreateWebhookRequest struct {
	ProjectID *uuid.UUID `json:"project_id"`
	URL       string     `json:"url"`
	Secret    string     `json:"secret"`
	Events    []string   `json:"events"`
}

// UpdateWebhookRequest defines mutable fields for an existing webhook subscription.
type UpdateWebhookRequest struct {
	URL      *string  `json:"url"`
	Events   []string `json:"events"`
	IsActive *bool    `json:"is_active"`
}

// WebhookService defines methods for managing webhook subscriptions and dispatching events.
type WebhookService interface {
	EventDispatcher
	CreateWebhook(ctx context.Context, userID uuid.UUID, req CreateWebhookRequest) (*WebhookResponse, error)
	ListWebhooks(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]WebhookResponse, error)
	GetWebhook(ctx context.Context, userID, webhookID uuid.UUID) (*WebhookResponse, error)
	UpdateWebhook(ctx context.Context, userID, webhookID uuid.UUID, req UpdateWebhookRequest) (*WebhookResponse, error)
	DeleteWebhook(ctx context.Context, userID, webhookID uuid.UUID) error
	TriggerTestPing(ctx context.Context, userID, webhookID uuid.UUID) error
	ListDeliveries(ctx context.Context, userID, webhookID uuid.UUID, limit, offset int32) ([]WebhookDeliveryResponse, error)
}

// EventDispatcher defines an interface for publishing system events to webhooks.
type EventDispatcher interface {
	DispatchEvent(ctx context.Context, actorID uuid.UUID, projectID *uuid.UUID, eventType string, data any) error
}

// EventDispatchable is implemented by services capable of dispatching webhook events.
type EventDispatchable interface {
	SetEventDispatcher(dispatcher EventDispatcher)
}

type webhookService struct {
	queries            repository.Querier
	distributor        worker.TaskDistributor
	allowLocalWebhooks bool
}

// NewWebhookService creates a new WebhookService instance.
func NewWebhookService(queries repository.Querier, distributor worker.TaskDistributor, allowLocalWebhooks bool) WebhookService {
	return &webhookService{
		queries:            queries,
		distributor:        distributor,
		allowLocalWebhooks: allowLocalWebhooks,
	}
}

// CreateWebhook validates parameters, generates an HMAC secret if omitted, and saves the subscription.
func (s *webhookService) CreateWebhook(ctx context.Context, userID uuid.UUID, req CreateWebhookRequest) (*WebhookResponse, error) {
	req.URL = strings.TrimSpace(req.URL)
	if err := webhook.ValidateURL(req.URL, s.allowLocalWebhooks); err != nil {
		return nil, err
	}

	if err := webhook.ValidateEvents(req.Events); err != nil {
		return nil, err
	}

	var projectUUID pgtype.UUID
	if req.ProjectID != nil {
		p, err := s.queries.GetProjectByID(ctx, *req.ProjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrProjectNotFound
			}
			return nil, err
		}
		if p.OwnerID != userID {
			member, err := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
				ProjectID: *req.ProjectID,
				UserID:    userID,
			})
			if err != nil || member.Role != "editor" {
				return nil, domain.ErrInsufficientProjectRole
			}
		}
		projectUUID = pgtype.UUID{Bytes: *req.ProjectID, Valid: true}
	}

	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		randomBytes := make([]byte, 24)
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, fmt.Errorf("failed to generate webhook secret: %w", err)
		}
		secret = "whsec_" + hex.EncodeToString(randomBytes)
	} else if len(secret) < 32 {
		return nil, domain.ErrInvalidWebhookSecret
	}

	wh, err := s.queries.CreateWebhook(ctx, repository.CreateWebhookParams{
		UserID:    userID,
		ProjectID: projectUUID,
		Url:       req.URL,
		Secret:    secret,
		Events:    req.Events,
		IsActive:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}

	resp := mapWebhookToResponse(wh)
	// Return full secret on creation so the caller can store it securely
	resp.Secret = wh.Secret
	return resp, nil
}

// ListWebhooks retrieves all webhooks belonging to a user or scoped to a project.
func (s *webhookService) ListWebhooks(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]WebhookResponse, error) {
	var webhooks []repository.Webhook
	var err error

	if projectID != nil {
		p, pErr := s.queries.GetProjectByID(ctx, *projectID)
		if pErr != nil {
			if errors.Is(pErr, pgx.ErrNoRows) {
				return nil, domain.ErrProjectNotFound
			}
			return nil, pErr
		}
		if p.OwnerID != userID {
			if _, mErr := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
				ProjectID: *projectID,
				UserID:    userID,
			}); mErr != nil {
				return nil, domain.ErrInsufficientProjectRole
			}
		}
		webhooks, err = s.queries.ListWebhooksByProjectID(ctx, pgtype.UUID{Bytes: *projectID, Valid: true})
	} else {
		webhooks, err = s.queries.ListWebhooksByUserID(ctx, userID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}

	responses := make([]WebhookResponse, 0, len(webhooks))
	for _, wh := range webhooks {
		responses = append(responses, *mapWebhookToResponse(wh))
	}
	return responses, nil
}

// GetWebhook returns a single webhook ensuring anti-IDOR authorization.
func (s *webhookService) GetWebhook(ctx context.Context, userID, webhookID uuid.UUID) (*WebhookResponse, error) {
	wh, err := s.queries.GetWebhookByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWebhookNotFound
		}
		return nil, err
	}

	if err := s.authorizeWebhookAccess(ctx, userID, wh); err != nil {
		return nil, err
	}

	return mapWebhookToResponse(wh), nil
}

// UpdateWebhook modifies webhook URL, subscribed events, or active status.
func (s *webhookService) UpdateWebhook(ctx context.Context, userID, webhookID uuid.UUID, req UpdateWebhookRequest) (*WebhookResponse, error) {
	wh, err := s.queries.GetWebhookByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWebhookNotFound
		}
		return nil, err
	}

	if err := s.authorizeWebhookAccess(ctx, userID, wh); err != nil {
		return nil, err
	}

	var urlParam pgtype.Text
	if req.URL != nil {
		trimmed := strings.TrimSpace(*req.URL)
		if err := webhook.ValidateURL(trimmed, s.allowLocalWebhooks); err != nil {
			return nil, err
		}
		urlParam = pgtype.Text{String: trimmed, Valid: true}
	}

	if req.Events != nil {
		if err := webhook.ValidateEvents(req.Events); err != nil {
			return nil, err
		}
	}

	var isActiveParam pgtype.Bool
	if req.IsActive != nil {
		isActiveParam = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}

	updated, err := s.queries.UpdateWebhook(ctx, repository.UpdateWebhookParams{
		ID:       webhookID,
		Url:      urlParam,
		Events:   req.Events,
		IsActive: isActiveParam,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}

	return mapWebhookToResponse(updated), nil
}

// DeleteWebhook removes a webhook subscription.
func (s *webhookService) DeleteWebhook(ctx context.Context, userID, webhookID uuid.UUID) error {
	wh, err := s.queries.GetWebhookByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrWebhookNotFound
		}
		return err
	}

	if err := s.authorizeWebhookAccess(ctx, userID, wh); err != nil {
		return err
	}

	if err := s.queries.DeleteWebhook(ctx, webhookID); err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	return nil
}

// TriggerTestPing enqueues a test event to verify the webhook endpoint connectivity.
func (s *webhookService) TriggerTestPing(ctx context.Context, userID, webhookID uuid.UUID) error {
	wh, err := s.queries.GetWebhookByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrWebhookNotFound
		}
		return err
	}

	if err := s.authorizeWebhookAccess(ctx, userID, wh); err != nil {
		return err
	}

	if s.distributor == nil {
		return nil
	}

	deliveryID := uuid.New()
	pingPayload, _ := json.Marshal(map[string]any{
		"id":         deliveryID.String(),
		"event":      "ping",
		"webhook_id": wh.ID.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"message":    "Ping test from Task Manager API webhook system",
	})

	return s.distributor.DistributeSendWebhook(ctx, worker.SendWebhookPayload{
		DeliveryID: deliveryID,
		WebhookID:  wh.ID,
		TargetURL:  wh.Url,
		Secret:     wh.Secret,
		EventType:  "ping",
		Payload:    pingPayload,
	})
}

// ListDeliveries retrieves the audit log of deliveries for a webhook subscription.
func (s *webhookService) ListDeliveries(ctx context.Context, userID, webhookID uuid.UUID, limit, offset int32) ([]WebhookDeliveryResponse, error) {
	wh, err := s.queries.GetWebhookByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWebhookNotFound
		}
		return nil, err
	}

	if err := s.authorizeWebhookAccess(ctx, userID, wh); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	deliveries, err := s.queries.ListWebhookDeliveries(ctx, repository.ListWebhookDeliveriesParams{
		WebhookID: webhookID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook deliveries: %w", err)
	}

	result := make([]WebhookDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		var respStatus *int32
		if d.ResponseStatus.Valid {
			statusVal := d.ResponseStatus.Int32
			respStatus = &statusVal
		}
		var respBody *string
		if d.ResponseBody.Valid {
			bodyVal := d.ResponseBody.String
			respBody = &bodyVal
		}
		var duration *int32
		if d.ExecutionDurationMs.Valid {
			dVal := d.ExecutionDurationMs.Int32
			duration = &dVal
		}

		result = append(result, WebhookDeliveryResponse{
			ID:                  d.ID,
			WebhookID:           d.WebhookID,
			EventType:           d.EventType,
			Payload:             string(d.Payload),
			ResponseStatus:      respStatus,
			ResponseBody:        respBody,
			ExecutionDurationMs: duration,
			Status:              d.Status,
			DeliveredAt:         d.DeliveredAt.Time,
		})
	}
	return result, nil
}

// DispatchEvent finds matching active webhooks and asynchronously enqueues delivery tasks.
func (s *webhookService) DispatchEvent(ctx context.Context, actorID uuid.UUID, projectID *uuid.UUID, eventType string, data any) error {
	if s.distributor == nil {
		return nil
	}

	var projectUUID pgtype.UUID
	if projectID != nil {
		projectUUID = pgtype.UUID{Bytes: *projectID, Valid: true}
	}

	webhooks, err := s.queries.ListActiveWebhooksForEvent(ctx, repository.ListActiveWebhooksForEventParams{
		UserID:    actorID,
		ProjectID: projectUUID,
		EventType: eventType,
	})
	if err != nil || len(webhooks) == 0 {
		return nil
	}

	for _, wh := range webhooks {
		deliveryID := uuid.New()
		envelopeBytes, err := json.Marshal(map[string]any{
			"id":        deliveryID.String(),
			"event":     eventType,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"actor_id":  actorID.String(),
			"data":      data,
		})
		if err != nil {
			return fmt.Errorf("failed to marshal webhook event envelope: %w", err)
		}

		_ = s.distributor.DistributeSendWebhook(ctx, worker.SendWebhookPayload{
			DeliveryID: deliveryID,
			WebhookID:  wh.ID,
			TargetURL:  wh.Url,
			Secret:     wh.Secret,
			EventType:  eventType,
			Payload:    envelopeBytes,
		})
	}

	return nil
}

func (s *webhookService) authorizeWebhookAccess(ctx context.Context, userID uuid.UUID, wh repository.Webhook) error {
	if wh.UserID == userID {
		return nil
	}

	if wh.ProjectID.Valid {
		p, err := s.queries.GetProjectByID(ctx, wh.ProjectID.Bytes)
		if err == nil {
			if p.OwnerID == userID {
				return nil
			}
			member, mErr := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
				ProjectID: wh.ProjectID.Bytes,
				UserID:    userID,
			})
			if mErr == nil && member.Role == "editor" {
				return nil
			}
		}
	}

	return domain.ErrWebhookNotFound
}

func mapWebhookToResponse(wh repository.Webhook) *WebhookResponse {
	var projectID *uuid.UUID
	if wh.ProjectID.Valid {
		id := uuid.UUID(wh.ProjectID.Bytes)
		projectID = &id
	}

	return &WebhookResponse{
		ID:        wh.ID,
		UserID:    wh.UserID,
		ProjectID: projectID,
		URL:       wh.Url,
		Events:    wh.Events,
		IsActive:  wh.IsActive,
		CreatedAt: wh.CreatedAt.Time,
		UpdatedAt: wh.UpdatedAt.Time,
	}
}
