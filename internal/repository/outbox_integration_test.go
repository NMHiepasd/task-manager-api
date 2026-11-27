//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"task-manager-api/internal/repository"
)

func TestIntegration_Outbox_Lifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Actor and Project
	email := fmt.Sprintf("outbox_user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_pwd_outbox",
		FullName:     "Outbox Test User",
		Role:         "user",
	})
	require.NoError(t, err)

	project, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     user.ID,
		Name:        "Outbox Project",
		Description: "Outbox Project Description",
		Color:       "#10b981",
	})
	require.NoError(t, err)

	taskID := uuid.New()
	payloadData, err := json.Marshal(map[string]any{
		"task_id": taskID.String(),
		"title":   "Outbox Lifecycle Task",
	})
	require.NoError(t, err)

	// 2. CreateOutboxEvent (Personal scope: ProjectID is NULL)
	event1, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		EventType:     "task.created",
		AggregateType: "task",
		AggregateID:   taskID,
		ActorID:       user.ID,
		ProjectID:     pgtype.UUID{Valid: false},
		Payload:       payloadData,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, event1.ID)
	assert.Equal(t, "task.created", event1.EventType)
	assert.Equal(t, "task", event1.AggregateType)
	assert.Equal(t, taskID, event1.AggregateID)
	assert.Equal(t, user.ID, event1.ActorID)
	assert.False(t, event1.ProjectID.Valid)
	assert.Equal(t, "pending", event1.Status)
	assert.Equal(t, int32(0), event1.RetryCount)
	assert.False(t, event1.LastError.Valid)
	assert.False(t, event1.ProcessedAt.Valid)

	// 3. CreateOutboxEvent (Project scope)
	event2, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		EventType:     "task.updated",
		AggregateType: "task",
		AggregateID:   taskID,
		ActorID:       user.ID,
		ProjectID:     pgtype.UUID{Bytes: project.ID, Valid: true},
		Payload:       payloadData,
	})
	require.NoError(t, err)
	assert.True(t, event2.ProjectID.Valid)
	assert.Equal(t, project.ID, uuid.UUID(event2.ProjectID.Bytes))

	// 4. FetchPendingOutboxEvents
	pending, err := testStore.FetchPendingOutboxEvents(ctx, repository.FetchPendingOutboxEventsParams{
		RetryCount: 5,
		Limit:      10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(pending), 2)

	found1, found2 := false, false
	for _, p := range pending {
		if p.ID == event1.ID {
			found1 = true
			assert.Equal(t, "task.created", p.EventType)
		}
		if p.ID == event2.ID {
			found2 = true
			assert.Equal(t, "task.updated", p.EventType)
		}
	}
	assert.True(t, found1, "event1 should be fetched")
	assert.True(t, found2, "event2 should be fetched")

	// 5. MarkOutboxEventPublished (Single)
	err = testStore.MarkOutboxEventPublished(ctx, event1.ID)
	require.NoError(t, err)

	refreshed1, err := testStore.GetOutboxEventByID(ctx, event1.ID)
	require.NoError(t, err)
	assert.Equal(t, "published", refreshed1.Status)
	assert.True(t, refreshed1.ProcessedAt.Valid)

	// 6. RecordOutboxEventRetry
	err = testStore.RecordOutboxEventRetry(ctx, repository.RecordOutboxEventRetryParams{
		ID:        event2.ID,
		LastError: pgtype.Text{String: "dial tcp: connection refused", Valid: true},
	})
	require.NoError(t, err)

	refreshed2, err := testStore.GetOutboxEventByID(ctx, event2.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), refreshed2.RetryCount)
	assert.Equal(t, "dial tcp: connection refused", refreshed2.LastError.String)
	assert.Equal(t, "pending", refreshed2.Status) // still pending after retry

	// 7. MarkOutboxEventFailed (Dead-Letter)
	err = testStore.MarkOutboxEventFailed(ctx, repository.MarkOutboxEventFailedParams{
		ID:        event2.ID,
		LastError: pgtype.Text{String: "max retries exceeded: permanent failure", Valid: true},
	})
	require.NoError(t, err)

	failedEvent2, err := testStore.GetOutboxEventByID(ctx, event2.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failedEvent2.Status)
	assert.Equal(t, "max retries exceeded: permanent failure", failedEvent2.LastError.String)
	assert.True(t, failedEvent2.ProcessedAt.Valid)

	// 8. MarkOutboxEventsPublished (Batch)
	event3, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		EventType:     "task.deleted",
		AggregateType: "task",
		AggregateID:   taskID,
		ActorID:       user.ID,
		ProjectID:     pgtype.UUID{Valid: false},
		Payload:       payloadData,
	})
	require.NoError(t, err)

	event4, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		EventType:     "comment.created",
		AggregateType: "comment",
		AggregateID:   uuid.New(),
		ActorID:       user.ID,
		ProjectID:     pgtype.UUID{Valid: false},
		Payload:       payloadData,
	})
	require.NoError(t, err)

	err = testStore.MarkOutboxEventsPublished(ctx, []uuid.UUID{event3.ID, event4.ID})
	require.NoError(t, err)

	refreshed3, err := testStore.GetOutboxEventByID(ctx, event3.ID)
	require.NoError(t, err)
	assert.Equal(t, "published", refreshed3.Status)

	refreshed4, err := testStore.GetOutboxEventByID(ctx, event4.ID)
	require.NoError(t, err)
	assert.Equal(t, "published", refreshed4.Status)

	// 9. DeleteOldProcessedOutboxEvents (Retention Pruning)
	// Query with future timestamp to simulate age > 7 days
	futureCutoff := time.Now().Add(1 * time.Hour)
	deletedCount, err := testStore.DeleteOldProcessedOutboxEvents(ctx, pgtype.Timestamptz{
		Time:  futureCutoff,
		Valid: true,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deletedCount, int64(3), "should prune published events (1, 3, 4)")

	// Failed event 2 must NOT be deleted by cleanup
	stillFailed, err := testStore.GetOutboxEventByID(ctx, event2.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stillFailed.Status)
}
