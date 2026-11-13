package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskDistributor provides an interface for enqueuing asynchronous tasks.
type TaskDistributor interface {
	DistributeCleanupExpiredTokens(ctx context.Context, opts ...asynq.Option) error
	DistributeScanDueTasks(ctx context.Context, opts ...asynq.Option) error
	DistributeDeadlineReminder(ctx context.Context, payload DeadlineReminderPayload, opts ...asynq.Option) error
	Close() error
}

// RedisTaskDistributor implements TaskDistributor using an Asynq Redis client.
type RedisTaskDistributor struct {
	client *asynq.Client
}

// NewRedisTaskDistributor creates a new Redis-backed TaskDistributor.
func NewRedisTaskDistributor(redisOpt asynq.RedisConnOpt) *RedisTaskDistributor {
	client := asynq.NewClient(redisOpt)
	return &RedisTaskDistributor{client: client}
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

// Close gracefully terminates the Asynq client connection.
func (d *RedisTaskDistributor) Close() error {
	return d.client.Close()
}
