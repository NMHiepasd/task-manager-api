package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"task-manager-api/internal/config"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/storage"
	"task-manager-api/pkg/webhook"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// TaskProcessor provides an interface to execute and process asynchronous background jobs.
type TaskProcessor interface {
	Start() error
	Shutdown()
	ProcessCleanupExpiredTokens(ctx context.Context, task *asynq.Task) error
	ProcessCleanupExpiredIdempotencyKeys(ctx context.Context, task *asynq.Task) error
	ProcessScanDueTasks(ctx context.Context, task *asynq.Task) error
	ProcessDeadlineReminder(ctx context.Context, task *asynq.Task) error
	ProcessSendWebhook(ctx context.Context, task *asynq.Task) error
	ProcessDeleteStorageObject(ctx context.Context, task *asynq.Task) error
}

// TaskStore defines the database access methods required by the background worker.
type TaskStore interface {
	DeleteExpiredRefreshTokens(ctx context.Context) error
	DeleteOldIdempotencyKeys(ctx context.Context, cutoff pgtype.Timestamptz) error
	ListTasksDueSoon(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error)
	CreateWebhookDelivery(ctx context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error)
}

// RedisTaskProcessor implements TaskProcessor using Asynq Server.
type RedisTaskProcessor struct {
	server      *asynq.Server
	mux         *asynq.ServeMux
	store       TaskStore
	distributor TaskDistributor
	storage     storage.Client
	log         *slog.Logger
	httpClient  *http.Client
}

// NewRedisTaskProcessor initializes a new worker server with priority queues and structured logging.
func NewRedisTaskProcessor(
	redisOpt asynq.RedisConnOpt,
	cfg *config.Config,
	store TaskStore,
	distributor TaskDistributor,
	log *slog.Logger,
	storageClient ...storage.Client,
) *RedisTaskProcessor {
	concurrency := cfg.WorkerConcurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: concurrency,
			Queues: map[string]int{
				QueueCritical: 6,
				QueueDefault:  3,
				QueueLow:      1,
			},
			ShutdownTimeout: 10 * time.Second,
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				retried, _ := asynq.GetRetryCount(ctx)
				maxRetry, _ := asynq.GetMaxRetry(ctx)
				log.Error("Asynq task failed during execution",
					"task_type", task.Type(),
					"retried", retried,
					"max_retry", maxRetry,
					"error", err,
				)
			}),
		},
	)

	mux := asynq.NewServeMux()
	var st storage.Client
	if len(storageClient) > 0 {
		st = storageClient[0]
	}
	processor := &RedisTaskProcessor{
		server:      server,
		mux:         mux,
		store:       store,
		distributor: distributor,
		storage:     st,
		log:         log,
		httpClient:  webhook.NewSafeHTTPClient(cfg.AllowLocalWebhooks || cfg.WebhookAllowPrivateHosts, 10*time.Second),
	}

	// Register task handlers
	mux.HandleFunc(TypeCleanupExpiredTokens, processor.ProcessCleanupExpiredTokens)
	mux.HandleFunc(TypeCleanupExpiredIdempotencyKeys, processor.ProcessCleanupExpiredIdempotencyKeys)
	mux.HandleFunc(TypeScanDueTasks, processor.ProcessScanDueTasks)
	mux.HandleFunc(TypeDeadlineReminder, processor.ProcessDeadlineReminder)
	mux.HandleFunc(TypeSendWebhook, processor.ProcessSendWebhook)
	mux.HandleFunc(TypeDeleteStorageObject, processor.ProcessDeleteStorageObject)

	return processor
}

// Start launches the worker server in background goroutines.
func (p *RedisTaskProcessor) Start() error {
	p.log.Info("Starting background task worker server")
	return p.server.Start(p.mux)
}

// Shutdown gracefully terminates the worker server, allowing in-flight tasks to complete.
func (p *RedisTaskProcessor) Shutdown() {
	p.log.Info("Shutting down background task worker server")
	p.server.Shutdown()
}

// ProcessCleanupExpiredTokens deletes expired refresh tokens from the database.
func (p *RedisTaskProcessor) ProcessCleanupExpiredTokens(ctx context.Context, task *asynq.Task) error {
	p.log.Info("Executing expired refresh tokens cleanup job", "task_type", task.Type())

	err := p.store.DeleteExpiredRefreshTokens(ctx)
	if err != nil {
		p.log.Error("Failed to delete expired refresh tokens", "error", err)
		return fmt.Errorf("cleanup expired tokens failed: %w", err)
	}

	p.log.Info("Expired refresh tokens cleaned up successfully")
	return nil
}

// ProcessCleanupExpiredIdempotencyKeys deletes old idempotency keys (older than 24h) from the database.
func (p *RedisTaskProcessor) ProcessCleanupExpiredIdempotencyKeys(ctx context.Context, task *asynq.Task) error {
	p.log.Info("Executing expired idempotency keys cleanup job", "task_type", task.Type())

	cutoff := time.Now().Add(-24 * time.Hour)
	err := p.store.DeleteOldIdempotencyKeys(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		p.log.Error("Failed to delete expired idempotency keys", "error", err)
		return fmt.Errorf("cleanup expired idempotency keys failed: %w", err)
	}

	p.log.Info("Expired idempotency keys cleaned up successfully")
	return nil
}

// ProcessScanDueTasks queries tasks approaching their deadline (within the next 24 hours) and fans out individual reminder tasks.
func (p *RedisTaskProcessor) ProcessScanDueTasks(ctx context.Context, task *asynq.Task) error {
	p.log.Info("Executing upcoming task deadline scanner", "task_type", task.Type())

	// Scan tasks due within next 24 hours
	threshold := time.Now().Add(24 * time.Hour)
	tasks, err := p.store.ListTasksDueSoon(ctx, repository.ListTasksDueSoonParams{
		DueDate: pgtype.Timestamptz{Time: threshold, Valid: true},
		Limit:   100,
	})
	if err != nil {
		p.log.Error("Failed to query tasks due soon", "error", err)
		return fmt.Errorf("scan due tasks query failed: %w", err)
	}

	p.log.Info("Found tasks approaching deadline", "count", len(tasks))

	for _, t := range tasks {
		payload := DeadlineReminderPayload{
			TaskID:         t.ID,
			RecipientEmail: t.RecipientEmail,
			RecipientName:  t.RecipientName,
			TaskTitle:      t.Title,
			DueDate:        t.DueDate.Time,
		}

		if err := p.distributor.DistributeDeadlineReminder(ctx, payload); err != nil {
			p.log.Warn("Failed to enqueue deadline reminder for task", "task_id", t.ID, "error", err)
		}
	}

	return nil
}

// ProcessDeadlineReminder handles sending an individual task deadline alert.
func (p *RedisTaskProcessor) ProcessDeadlineReminder(_ context.Context, task *asynq.Task) error {
	var payload DeadlineReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		p.log.Error("Failed to unmarshal deadline reminder payload", "error", err)
		return fmt.Errorf("invalid deadline reminder payload: %w", asynq.SkipRetry)
	}

	// Dispatch deadline alert (e.g., SMTP/SendGrid/Push notification simulation)
	p.log.Info("Dispatched task deadline reminder notification",
		"task_id", payload.TaskID.String(),
		"recipient_email", payload.RecipientEmail,
		"recipient_name", payload.RecipientName,
		"task_title", payload.TaskTitle,
		"due_date", payload.DueDate.Format(time.RFC3339),
	)

	return nil
}

// ProcessSendWebhook delivers an HTTP webhook notification to the target subscriber.
func (p *RedisTaskProcessor) ProcessSendWebhook(ctx context.Context, task *asynq.Task) error {
	var payload SendWebhookPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		p.log.Error("Failed to unmarshal send webhook payload", "error", err)
		return fmt.Errorf("invalid send webhook payload: %w", asynq.SkipRetry)
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.TargetURL, bytes.NewReader(payload.Payload))
	if err != nil {
		p.log.Error("Failed to create webhook request", "error", err, "target_url", payload.TargetURL)
		return fmt.Errorf("failed to create webhook request: %w", asynq.SkipRetry)
	}

	deliveryID := payload.DeliveryID
	if deliveryID == uuid.Nil {
		deliveryID = uuid.New()
	}
	deliveryIDStr := deliveryID.String()

	tsStr := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhook.SignV2(payload.Secret, tsStr, deliveryIDStr, payload.Payload)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", payload.EventType)
	req.Header.Set("X-Webhook-Id", deliveryIDStr)
	req.Header.Set("X-Webhook-Timestamp", tsStr)
	req.Header.Set("X-Webhook-Signature", "v1="+sig)
	req.Header.Set("User-Agent", "TaskManager-Webhook/2.0")

	resp, err := p.httpClient.Do(req)
	durationMs := int32(time.Since(start).Milliseconds()) //nolint:gosec // G115: Delivery timeout is capped at 10s, fits in int32

	if err != nil {
		p.log.Warn("Webhook delivery failed with network error", "target_url", payload.TargetURL, "error", err)
		_, _ = p.store.CreateWebhookDelivery(ctx, repository.CreateWebhookDeliveryParams{
			WebhookID:           payload.WebhookID,
			EventType:           payload.EventType,
			Payload:             payload.Payload,
			ResponseStatus:      pgtype.Int4{Valid: false},
			ResponseBody:        pgtype.Text{String: err.Error(), Valid: true},
			ExecutionDurationMs: pgtype.Int4{Int32: durationMs, Valid: true},
			Status:              "failed",
		})
		if errors.Is(err, webhook.ErrBlockedIP) {
			return fmt.Errorf("webhook delivery blocked by SSRF protection: %w", asynq.SkipRetry)
		}
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	respBody := string(bodyBytes)

	status := "success"
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status = "failed"
	}

	_, dbErr := p.store.CreateWebhookDelivery(ctx, repository.CreateWebhookDeliveryParams{
		WebhookID:           payload.WebhookID,
		EventType:           payload.EventType,
		Payload:             payload.Payload,
		ResponseStatus:      pgtype.Int4{Int32: int32(resp.StatusCode), Valid: true}, //nolint:gosec // G115: HTTP status code is bounded between 100 and 599
		ResponseBody:        pgtype.Text{String: respBody, Valid: true},
		ExecutionDurationMs: pgtype.Int4{Int32: durationMs, Valid: true},
		Status:              status,
	})
	if dbErr != nil {
		p.log.Debug("Could not record webhook delivery (webhook may have been removed)", "error", dbErr)
	}

	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook endpoint returned server error %d", resp.StatusCode)
	}

	p.log.Info("Webhook delivery completed",
		"webhook_id", payload.WebhookID.String(),
		"event_type", payload.EventType,
		"status_code", resp.StatusCode,
		"duration_ms", durationMs,
	)

	return nil
}

// ProcessDeleteStorageObject executes background deletion of an orphan object from storage.
func (p *RedisTaskProcessor) ProcessDeleteStorageObject(ctx context.Context, task *asynq.Task) error {
	var payload DeleteStorageObjectPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		p.log.Error("Failed to unmarshal delete storage object payload", "error", err)
		return fmt.Errorf("invalid delete storage object payload: %w", asynq.SkipRetry)
	}

	if payload.StorageKey == "" {
		p.log.Warn("Delete storage object task has empty storage key, skipping")
		return nil
	}

	if p.storage == nil {
		p.log.Error("Storage client is not configured for worker processor", "storage_key", payload.StorageKey)
		return errors.New("storage client not configured on worker processor")
	}

	err := p.storage.Delete(ctx, payload.StorageKey)
	if err != nil {
		if storage.IsNotFoundError(err) {
			p.log.Info("Storage object does not exist, skipping deletion retry",
				"storage_key", payload.StorageKey,
				"error", err,
			)
			return nil
		}
		p.log.Warn("Failed to delete storage object, will retry",
			"storage_key", payload.StorageKey,
			"error", err,
		)
		return fmt.Errorf("failed to delete storage object %s: %w", payload.StorageKey, err)
	}

	p.log.Info("Successfully deleted orphan storage object", "storage_key", payload.StorageKey)
	return nil
}
