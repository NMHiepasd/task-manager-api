package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"task-manager-api/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// OutboxStore defines database interactions required by the outbox relay worker.
type OutboxStore interface {
	ClaimOutboxEvents(ctx context.Context, arg repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error)
	ListActiveWebhooksForEvent(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error)
	MarkOutboxEventsPublished(ctx context.Context, ids []uuid.UUID) error
	RecordOutboxEventRetry(ctx context.Context, arg repository.RecordOutboxEventRetryParams) error
	MarkOutboxEventFailed(ctx context.Context, arg repository.MarkOutboxEventFailedParams) error
}

// OutboxRelayOption provides functional configuration for OutboxRelay.
type OutboxRelayOption func(*OutboxRelay)

// WithPollInterval sets the interval between polling passes.
func WithPollInterval(interval time.Duration) OutboxRelayOption {
	return func(r *OutboxRelay) {
		if interval > 0 {
			r.pollInterval = interval
		}
	}
}

// WithBatchSize sets maximum number of pending events polled in a single batch.
func WithBatchSize(size int32) OutboxRelayOption {
	return func(r *OutboxRelay) {
		if size > 0 {
			r.batchSize = size
		}
	}
}

// WithMaxRetries sets the failure threshold before an outbox event transitions to 'failed'.
func WithMaxRetries(retries int32) OutboxRelayOption {
	return func(r *OutboxRelay) {
		if retries > 0 {
			r.maxRetries = retries
		}
	}
}

// OutboxRelay periodically polls pending events from outbox_events and enqueues tasks to Asynq.
type OutboxRelay struct {
	store        OutboxStore
	distributor  TaskDistributor
	log          *slog.Logger
	pollInterval time.Duration
	batchSize    int32
	maxRetries   int32

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewOutboxRelay initializes an OutboxRelay.
func NewOutboxRelay(store OutboxStore, distributor TaskDistributor, log *slog.Logger, opts ...OutboxRelayOption) *OutboxRelay {
	r := &OutboxRelay{
		store:        store,
		distributor:  distributor,
		log:          log,
		pollInterval: 1 * time.Second,
		batchSize:    50,
		maxRetries:   5,
		stopCh:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Start begins periodic polling in a background goroutine.
func (r *OutboxRelay) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.log.Info("Starting outbox relay worker", "poll_interval", r.pollInterval, "batch_size", r.batchSize)

		ticker := time.NewTicker(r.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.stopCh:
				r.log.Info("Stopping outbox relay worker")
				return
			case <-ctx.Done():
				r.log.Info("Context cancelled, stopping outbox relay worker")
				return
			case <-ticker.C:
				if _, err := r.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
					r.log.Error("Error during outbox relay batch processing", "error", err)
				}
			}
		}
	}()
}

// Stop terminates the polling loop gracefully.
func (r *OutboxRelay) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// ProcessBatch polls a single batch of pending outbox events and dispatches them to webhooks.
// Returns the number of events processed and any non-fatal error encountered.
func (r *OutboxRelay) ProcessBatch(ctx context.Context) (int, error) {
	if r.distributor == nil {
		return 0, nil
	}

	events, err := r.store.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
		MaxRetries: r.maxRetries,
		BatchLimit: r.batchSize,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to claim pending outbox events: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	publishedIDs := make([]uuid.UUID, 0, len(events))

	for _, event := range events {
		webhooks, err := r.store.ListActiveWebhooksForEvent(ctx, repository.ListActiveWebhooksForEventParams{
			UserID:    event.ActorID,
			ProjectID: event.ProjectID,
			EventType: event.EventType,
		})
		if err != nil {
			r.log.Error("Failed to list active webhooks for outbox event", "event_id", event.ID, "error", err)
			r.handleEventFailure(ctx, event, err)
			continue
		}

		eventTimestamp := event.CreatedAt.Time
		if !event.CreatedAt.Valid || eventTimestamp.IsZero() {
			eventTimestamp = time.Now()
		}

		var enqueueErr error
		for _, wh := range webhooks {
			deliveryID := uuid.NewSHA1(event.ID, wh.ID[:])
			envelopeBytes, err := json.Marshal(map[string]any{
				"id":        deliveryID.String(),
				"event":     event.EventType,
				"timestamp": eventTimestamp.UTC().Format(time.RFC3339),
				"actor_id":  event.ActorID.String(),
				"data":      json.RawMessage(event.Payload),
			})
			if err != nil {
				enqueueErr = fmt.Errorf("failed to marshal webhook payload envelope: %w", err)
				break
			}

			err = r.distributor.DistributeSendWebhook(ctx, SendWebhookPayload{
				DeliveryID: deliveryID,
				WebhookID:  wh.ID,
				TargetURL:  wh.Url,
				EventType:  event.EventType,
				Payload:    envelopeBytes,
			}, asynq.TaskID(deliveryID.String()))
			if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
				enqueueErr = fmt.Errorf("failed to enqueue webhook task: %w", err)
				break
			}
		}

		if enqueueErr != nil {
			r.log.Warn("Failed to dispatch outbox event to webhook queue", "event_id", event.ID, "error", enqueueErr)
			r.handleEventFailure(ctx, event, enqueueErr)
			continue
		}

		publishedIDs = append(publishedIDs, event.ID)
	}

	if len(publishedIDs) > 0 {
		if err := r.store.MarkOutboxEventsPublished(ctx, publishedIDs); err != nil {
			r.log.Error("Failed to mark outbox events as published", "count", len(publishedIDs), "error", err)
			return len(publishedIDs), fmt.Errorf("failed to mark outbox events published: %w", err)
		}
		r.log.Debug("Published outbox events batch", "count", len(publishedIDs))
	}

	return len(publishedIDs), nil
}

func (r *OutboxRelay) handleEventFailure(ctx context.Context, event repository.ClaimOutboxEventsRow, failureErr error) {
	newRetryCount := event.RetryCount + 1
	errMsg := failureErr.Error()

	if newRetryCount >= r.maxRetries {
		r.log.Error("Outbox event reached maximum retries, marking failed (dead-letter)",
			"event_id", event.ID,
			"retry_count", newRetryCount,
			"error", errMsg,
		)
		_ = r.store.MarkOutboxEventFailed(ctx, repository.MarkOutboxEventFailedParams{
			ID:        event.ID,
			LastError: pgtype.Text{String: errMsg, Valid: true},
		})
	} else {
		_ = r.store.RecordOutboxEventRetry(ctx, repository.RecordOutboxEventRetryParams{
			ID:        event.ID,
			LastError: pgtype.Text{String: errMsg, Valid: true},
		})
	}
}
