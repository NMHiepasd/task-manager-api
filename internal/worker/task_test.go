package worker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCleanupExpiredTokensTask(t *testing.T) {
	task := NewCleanupExpiredTokensTask()
	require.NotNil(t, task)
	assert.Equal(t, TypeCleanupExpiredTokens, task.Type())
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
