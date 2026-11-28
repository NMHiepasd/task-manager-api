//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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

	// 4. ClaimOutboxEvents (transitions 'pending' -> 'processing')
	claimed, err := testStore.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
		MaxRetries: 5,
		BatchLimit: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(claimed), 2)

	found1, found2 := false, false
	for _, p := range claimed {
		if p.ID == event1.ID {
			found1 = true
			assert.Equal(t, "task.created", p.EventType)
			assert.Equal(t, "processing", p.Status)
		}
		if p.ID == event2.ID {
			found2 = true
			assert.Equal(t, "task.updated", p.EventType)
			assert.Equal(t, "processing", p.Status)
		}
	}
	assert.True(t, found1, "event1 should be claimed")
	assert.True(t, found2, "event2 should be claimed")

	// 5. MarkOutboxEventPublished (Single, on 'processing' row)
	err = testStore.MarkOutboxEventPublished(ctx, event1.ID)
	require.NoError(t, err)

	refreshed1, err := testStore.GetOutboxEventByID(ctx, event1.ID)
	require.NoError(t, err)
	assert.Equal(t, "published", refreshed1.Status)
	assert.True(t, refreshed1.ProcessedAt.Valid)

	// 6. RecordOutboxEventRetry (transitions 'processing' -> 'pending' with retry_count+1)
	err = testStore.RecordOutboxEventRetry(ctx, repository.RecordOutboxEventRetryParams{
		ID:        event2.ID,
		LastError: pgtype.Text{String: "dial tcp: connection refused", Valid: true},
	})
	require.NoError(t, err)

	refreshed2, err := testStore.GetOutboxEventByID(ctx, event2.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), refreshed2.RetryCount)
	assert.Equal(t, "dial tcp: connection refused", refreshed2.LastError.String)
	assert.Equal(t, "pending", refreshed2.Status) // back to pending after retry

	// Re-claim event2 so it is 'processing' again before marking failed
	_, err = testStore.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
		MaxRetries: 5,
		BatchLimit: 10,
	})
	require.NoError(t, err)

	// 7. MarkOutboxEventFailed (Dead-Letter, on 'processing' row)
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

	_, err = testStore.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
		MaxRetries: 5,
		BatchLimit: 10,
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

func TestIntegration_Outbox_ClaimAndLease(t *testing.T) {
	ctx := context.Background()

	email := fmt.Sprintf("outbox_claim_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_pwd_claim",
		FullName:     "Outbox Claim User",
		Role:         "user",
	})
	require.NoError(t, err)

	payloadData, err := json.Marshal(map[string]any{"event": "claim_test"})
	require.NoError(t, err)

	// Create 10 pending events
	const totalEvents = 10
	createdIDs := make(map[uuid.UUID]bool, totalEvents)
	for i := 0; i < totalEvents; i++ {
		ev, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.created",
			AggregateType: "task",
			AggregateID:   uuid.New(),
			ActorID:       user.ID,
			ProjectID:     pgtype.UUID{Valid: false},
			Payload:       payloadData,
		})
		require.NoError(t, err)
		createdIDs[ev.ID] = true
	}

	// 1. Two concurrent goroutines claiming events simultaneously must receive disjoint sets
	var wg sync.WaitGroup
	results := make([][]repository.ClaimOutboxEventsRow, 2)
	errs := make([]error, 2)

	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rows, claimErr := testStore.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
				MaxRetries: 5,
				BatchLimit: 6,
			})
			results[idx] = rows
			errs[idx] = claimErr
		}(g)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	claimedBy0 := make(map[uuid.UUID]bool)
	for _, r := range results[0] {
		assert.Equal(t, "processing", r.Status)
		claimedBy0[r.ID] = true
	}

	claimedBy1 := make(map[uuid.UUID]bool)
	for _, r := range results[1] {
		assert.Equal(t, "processing", r.Status)
		assert.False(t, claimedBy0[r.ID], "goroutine 1 and goroutine 2 must claim disjoint event sets")
		claimedBy1[r.ID] = true
	}

	totalClaimed := len(claimedBy0) + len(claimedBy1)
	assert.Equal(t, totalEvents, totalClaimed, "all 10 events must be claimed exactly once across both goroutines")

	// 2. Verify state transition guards: pending row cannot be marked published directly
	pendingEv, err := testStore.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		EventType:     "task.updated",
		AggregateType: "task",
		AggregateID:   uuid.New(),
		ActorID:       user.ID,
		ProjectID:     pgtype.UUID{Valid: false},
		Payload:       payloadData,
	})
	require.NoError(t, err)

	err = testStore.MarkOutboxEventPublished(ctx, pendingEv.ID)
	require.NoError(t, err)
	afterGuard, err := testStore.GetOutboxEventByID(ctx, pendingEv.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", afterGuard.Status, "MarkOutboxEventPublished must only affect 'processing' rows")

	// Claim pendingEv so it becomes 'processing'
	claimedSingle, err := testStore.ClaimOutboxEvents(ctx, repository.ClaimOutboxEventsParams{
		MaxRetries: 5,
		BatchLimit: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, claimedSingle)

	// 3. ReclaimStaleOutboxEvents: active lease (120s) must NOT reclaim freshly claimed rows
	reclaimedActive, err := testStore.ReclaimStaleOutboxEvents(ctx, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reclaimedActive)

	// Sleep briefly and reclaim with 0s lease to simulate expired lease
	time.Sleep(20 * time.Millisecond)
	reclaimedExpired, err := testStore.ReclaimStaleOutboxEvents(ctx, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, reclaimedExpired, int64(1))

	reclaimedCheck, err := testStore.GetOutboxEventByID(ctx, pendingEv.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", reclaimedCheck.Status)
	assert.False(t, reclaimedCheck.LockedAt.Valid)
}
