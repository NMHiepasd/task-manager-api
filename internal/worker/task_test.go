package worker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCleanupExpiredTokensTask(t *testing.T) {
	task := NewCleanupExpiredTokensTask()
	require.NotNil(t, task)
	assert.Equal(t, TypeCleanupExpiredTokens, task.Type())
	assert.Empty(t, task.Payload())
}

func TestNewCleanupExpiredIdempotencyKeysTask(t *testing.T) {
	task := NewCleanupExpiredIdempotencyKeysTask()
	require.NotNil(t, task)
	assert.Equal(t, TypeCleanupExpiredIdempotencyKeys, task.Type())
	assert.Empty(t, task.Payload())
}

func TestNewScanDueTasksTask(t *testing.T) {
	task := NewScanDueTasksTask()
	require.NotNil(t, task)
	assert.Equal(t, TypeScanDueTasks, task.Type())
	assert.Empty(t, task.Payload())
}

func TestNewDeadlineReminderTask(t *testing.T) {
	payload := DeadlineReminderPayload{
		TaskID:         uuid.New(),
		RecipientEmail: "user@example.com",
		RecipientName:  "Test User",
		TaskTitle:      "Complete Task 10.2",
		DueDate:        time.Now().Add(2 * time.Hour).Truncate(time.Second),
	}

	task, err := NewDeadlineReminderTask(payload)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TypeDeadlineReminder, task.Type())

	var decoded DeadlineReminderPayload
	err = json.Unmarshal(task.Payload(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, payload.TaskID, decoded.TaskID)
	assert.Equal(t, payload.RecipientEmail, decoded.RecipientEmail)
	assert.Equal(t, payload.RecipientName, decoded.RecipientName)
	assert.Equal(t, payload.TaskTitle, decoded.TaskTitle)
	assert.True(t, payload.DueDate.Equal(decoded.DueDate))
}

func TestNewSendWebhookTask(t *testing.T) {
	//nolint:gosec // G101: Test payload with dummy secret
	payload := SendWebhookPayload{
		DeliveryID: uuid.New(),
		WebhookID:  uuid.New(),
		TargetURL:  "https://example.com/webhook",
		Secret:     "dummy-webhook-secret-string-12345",
		EventType:  "task:created",
		Payload:    []byte(`{"id":"task-123"}`),
	}

	task, err := NewSendWebhookTask(payload)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TypeSendWebhook, task.Type())

	var decoded SendWebhookPayload
	err = json.Unmarshal(task.Payload(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, payload.DeliveryID, decoded.DeliveryID)
	assert.Equal(t, payload.WebhookID, decoded.WebhookID)
	assert.Equal(t, payload.TargetURL, decoded.TargetURL)
	assert.Equal(t, payload.Secret, decoded.Secret)
	assert.Equal(t, payload.EventType, decoded.EventType)
	assert.Equal(t, payload.Payload, decoded.Payload)
}

func TestNewRedisTaskDistributor_DefaultAndCustomRetry(t *testing.T) {
	dDefault := NewRedisTaskDistributor(asynq.RedisClientOpt{Addr: "localhost:6379"})
	assert.Equal(t, 6, dDefault.webhookMaxRetry)

	dCustom := NewRedisTaskDistributor(asynq.RedisClientOpt{Addr: "localhost:6379"}, 10)
	assert.Equal(t, 10, dCustom.webhookMaxRetry)

	dInvalid := NewRedisTaskDistributor(asynq.RedisClientOpt{Addr: "localhost:6379"}, -1)
	assert.Equal(t, 6, dInvalid.webhookMaxRetry)
}

func TestNewDeleteStorageObjectTask(t *testing.T) {
	payload := DeleteStorageObjectPayload{
		StorageKey: "tasks/987/sample.png",
	}

	task, err := NewDeleteStorageObjectTask(payload)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TypeDeleteStorageObject, task.Type())

	var decoded DeleteStorageObjectPayload
	err = json.Unmarshal(task.Payload(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, payload.StorageKey, decoded.StorageKey)
}
