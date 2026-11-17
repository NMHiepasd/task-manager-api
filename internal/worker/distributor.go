package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskDistributor provides an interface for enqueuing asynchronous tasks.
type TaskDistributor interface {
	DistributeCleanupExpiredTokens(ctx context.Context, opts ...asynq.Option) error
	DistributeCleanupExpiredIdempotencyKeys(ctx context.Context, opts ...asynq.Option) error
	DistributeScanDueTasks(ctx context.Context, opts ...asynq.Option) error
	DistributeDeadlineReminder(ctx context.Context, payload DeadlineReminderPayload, opts ...asynq.Option) error
	DistributeSendWebhook(ctx context.Context, payload SendWebhookPayload, opts ...asynq.Option) error
	DistributeDeleteStorageObject(ctx context.Context, payload DeleteStorageObjectPayload, opts ...asynq.Option) error
	Close() error
}

// RedisTaskDistributor implements TaskDistributor using an Asynq Redis client.
type RedisTaskDistributor struct {
	client          *asynq.Client
	webhookMaxRetry int
}

// NewRedisTaskDistributor creates a new Redis-backed TaskDistributor.
func NewRedisTaskDistributor(redisOpt asynq.RedisConnOpt, webhookMaxRetry ...int) *RedisTaskDistributor {
	retry := 6
	if len(webhookMaxRetry) > 0 && webhookMaxRetry[0] > 0 {
		retry = webhookMaxRetry[0]
	}
	client := asynq.NewClient(redisOpt)
	return &RedisTaskDistributor{
		client:          client,
		webhookMaxRetry: retry,
	}
}

// DistributeCleanupExpiredTokens enqueues a cleanup task.
func (d *RedisTaskDistributor) DistributeCleanupExpiredTokens(ctx context.Context, opts ...asynq.Option) error {
	task := NewCleanupExpiredTokensTask()
	_, err := d.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue cleanup expired tokens task: %w", err)
	}
	return nil
}

// DistributeCleanupExpiredIdempotencyKeys enqueues an idempotency keys cleanup task.
func (d *RedisTaskDistributor) DistributeCleanupExpiredIdempotencyKeys(ctx context.Context, opts ...asynq.Option) error {
	task := NewCleanupExpiredIdempotencyKeysTask()
	_, err := d.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue cleanup expired idempotency keys task: %w", err)
	}
	return nil
}

// DistributeScanDueTasks enqueues a scan task.
func (d *RedisTaskDistributor) DistributeScanDueTasks(ctx context.Context, opts ...asynq.Option) error {
	task := NewScanDueTasksTask()
	_, err := d.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue scan due tasks task: %w", err)
	}
	return nil
}

// DistributeDeadlineReminder enqueues an individual user deadline notification.
func (d *RedisTaskDistributor) DistributeDeadlineReminder(ctx context.Context, payload DeadlineReminderPayload, opts ...asynq.Option) error {
	task, err := NewDeadlineReminderTask(payload)
	if err != nil {
		return err
	}
	_, err = d.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue deadline reminder task: %w", err)
	}
	return nil
}

// DistributeSendWebhook enqueues a webhook delivery task with retry limits and execution timeout.
func (d *RedisTaskDistributor) DistributeSendWebhook(ctx context.Context, payload SendWebhookPayload, opts ...asynq.Option) error {
	task, err := NewSendWebhookTask(payload)
	if err != nil {
		return err
	}

	maxRetry := d.webhookMaxRetry
	if maxRetry <= 0 {
		maxRetry = 6
	}

	defaultOpts := []asynq.Option{
		asynq.MaxRetry(maxRetry),
		asynq.Timeout(15 * time.Second),
	}
	allOpts := append(defaultOpts, opts...)

	_, err = d.client.EnqueueContext(ctx, task, allOpts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue send webhook task: %w", err)
	}
	return nil
}

// DistributeDeleteStorageObject enqueues an asynchronous orphan object cleanup task.
func (d *RedisTaskDistributor) DistributeDeleteStorageObject(ctx context.Context, payload DeleteStorageObjectPayload, opts ...asynq.Option) error {
	task, err := NewDeleteStorageObjectTask(payload)
	if err != nil {
		return err
	}

	defaultOpts := []asynq.Option{
		asynq.MaxRetry(5),
		asynq.Timeout(30 * time.Second),
	}
	allOpts := append(defaultOpts, opts...)

	_, err = d.client.EnqueueContext(ctx, task, allOpts...)
	if err != nil {
		return fmt.Errorf("failed to enqueue delete storage object task: %w", err)
	}
	return nil
}

// Close gracefully terminates the Asynq client connection.
func (d *RedisTaskDistributor) Close() error {
	return d.client.Close()
}
