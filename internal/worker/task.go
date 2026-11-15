package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Queue priorities and names
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// Task type constants
const (
	// TypeCleanupExpiredTokens identifies the background task for pruning expired refresh tokens.
	//nolint:gosec // G101: Task identifier contains the word token, not a credential
	TypeCleanupExpiredTokens          = "task:cleanup_expired_tokens"
	TypeCleanupExpiredIdempotencyKeys = "task:cleanup_expired_idempotency_keys"
	TypeScanDueTasks                  = "task:scan_due_tasks"
	TypeDeadlineReminder              = "task:deadline_reminder"
	TypeSendWebhook                   = "task:send_webhook"
)

// SendWebhookPayload defines the information needed to deliver an event to a webhook endpoint.
type SendWebhookPayload struct {
	WebhookID uuid.UUID `json:"webhook_id"`
	TargetURL string    `json:"target_url"`
	Secret    string    `json:"secret"`
	EventType string    `json:"event_type"`
	Payload   []byte    `json:"payload"`
}

// DeadlineReminderPayload defines the data needed to notify a user of an upcoming task deadline.
type DeadlineReminderPayload struct {
	TaskID         uuid.UUID `json:"task_id"`
	RecipientEmail string    `json:"recipient_email"`
	RecipientName  string    `json:"recipient_name"`
	TaskTitle      string    `json:"task_title"`
	DueDate        time.Time `json:"due_date"`
}

// NewCleanupExpiredTokensTask creates an Asynq task to prune expired refresh tokens from the database.
func NewCleanupExpiredTokensTask() *asynq.Task {
	return asynq.NewTask(TypeCleanupExpiredTokens, nil, asynq.Queue(QueueLow), asynq.MaxRetry(3))
}

// NewCleanupExpiredIdempotencyKeysTask creates an Asynq task to prune expired idempotency keys from the database.
func NewCleanupExpiredIdempotencyKeysTask() *asynq.Task {
	return asynq.NewTask(TypeCleanupExpiredIdempotencyKeys, nil, asynq.Queue(QueueLow), asynq.MaxRetry(3))
}

// NewScanDueTasksTask creates an Asynq task to scan for tasks approaching their deadline and fan-out reminder tasks.
func NewScanDueTasksTask() *asynq.Task {
	return asynq.NewTask(TypeScanDueTasks, nil, asynq.Queue(QueueDefault), asynq.MaxRetry(3))
}

// NewDeadlineReminderTask creates an Asynq task to send a deadline reminder to a user.
func NewDeadlineReminderTask(payload DeadlineReminderPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deadline reminder payload: %w", err)
	}
	return asynq.NewTask(TypeDeadlineReminder, data, asynq.Queue(QueueCritical), asynq.MaxRetry(5)), nil
}

// NewSendWebhookTask creates an Asynq task to deliver an HTTP webhook notification.
func NewSendWebhookTask(payload SendWebhookPayload) (*asynq.Task, error) {
	//nolint:gosec // G117: Webhook secret must be marshaled into worker task payload for HMAC computation
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal send webhook payload: %w", err)
	}
	return asynq.NewTask(TypeSendWebhook, data, asynq.Queue(QueueDefault), asynq.MaxRetry(5)), nil
}
