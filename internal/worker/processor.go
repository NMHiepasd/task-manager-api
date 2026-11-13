package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"task-manager-api/internal/config"
	"task-manager-api/internal/repository"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// TaskProcessor provides an interface to execute and process asynchronous background jobs.
type TaskProcessor interface {
	Start() error
	Shutdown()
	ProcessCleanupExpiredTokens(ctx context.Context, task *asynq.Task) error
	ProcessScanDueTasks(ctx context.Context, task *asynq.Task) error
	ProcessDeadlineReminder(ctx context.Context, task *asynq.Task) error
}

// TaskStore defines the database access methods required by the background worker.
type TaskStore interface {
	DeleteExpiredRefreshTokens(ctx context.Context) error
	ListTasksDueSoon(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error)
}

// RedisTaskProcessor implements TaskProcessor using Asynq Server.
type RedisTaskProcessor struct {
	server      *asynq.Server
	mux         *asynq.ServeMux
	store       TaskStore
	distributor TaskDistributor
	log         *slog.Logger
}

// NewRedisTaskProcessor initializes a new worker server with priority queues and structured logging.
func NewRedisTaskProcessor(
	redisOpt asynq.RedisConnOpt,
	cfg *config.Config,
	store TaskStore,
	distributor TaskDistributor,
	log *slog.Logger,
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
	processor := &RedisTaskProcessor{
		server:      server,
		mux:         mux,
		store:       store,
		distributor: distributor,
		log:         log,
	}

	// Register task handlers
	mux.HandleFunc(TypeCleanupExpiredTokens, processor.ProcessCleanupExpiredTokens)
	mux.HandleFunc(TypeScanDueTasks, processor.ProcessScanDueTasks)
	mux.HandleFunc(TypeDeadlineReminder, processor.ProcessDeadlineReminder)

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
