package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOutboxStore struct {
	mu sync.Mutex

	claimEventsFn        func(ctx context.Context, arg repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error)
	reclaimStaleFn       func(ctx context.Context, leaseSeconds float64) (int64, error)
	listActiveWebhooksFn func(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error)
	markPublishedFn      func(ctx context.Context, ids []uuid.UUID) error
	recordRetryFn        func(ctx context.Context, arg repository.RecordOutboxEventRetryParams) error
	markFailedFn         func(ctx context.Context, arg repository.MarkOutboxEventFailedParams) error

	reclaimCalls []float64
	publishedIDs []uuid.UUID
	retryCalls   []repository.RecordOutboxEventRetryParams
	failedCalls  []repository.MarkOutboxEventFailedParams
}

func (m *mockOutboxStore) ClaimOutboxEvents(ctx context.Context, arg repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
	if m.claimEventsFn != nil {
		return m.claimEventsFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockOutboxStore) ReclaimStaleOutboxEvents(ctx context.Context, leaseSeconds float64) (int64, error) {
	m.mu.Lock()
	m.reclaimCalls = append(m.reclaimCalls, leaseSeconds)
	m.mu.Unlock()

	if m.reclaimStaleFn != nil {
		return m.reclaimStaleFn(ctx, leaseSeconds)
	}
	return 0, nil
}

func (m *mockOutboxStore) ListActiveWebhooksForEvent(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
	if m.listActiveWebhooksFn != nil {
		return m.listActiveWebhooksFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockOutboxStore) MarkOutboxEventsPublished(ctx context.Context, ids []uuid.UUID) error {
	m.mu.Lock()
	m.publishedIDs = append(m.publishedIDs, ids...)
	m.mu.Unlock()

	if m.markPublishedFn != nil {
		return m.markPublishedFn(ctx, ids)
	}
	return nil
}

func (m *mockOutboxStore) RecordOutboxEventRetry(ctx context.Context, arg repository.RecordOutboxEventRetryParams) error {
	m.mu.Lock()
	m.retryCalls = append(m.retryCalls, arg)
	m.mu.Unlock()

	if m.recordRetryFn != nil {
		return m.recordRetryFn(ctx, arg)
	}
	return nil
}

func (m *mockOutboxStore) MarkOutboxEventFailed(ctx context.Context, arg repository.MarkOutboxEventFailedParams) error {
	m.mu.Lock()
	m.failedCalls = append(m.failedCalls, arg)
	m.mu.Unlock()

	if m.markFailedFn != nil {
		return m.markFailedFn(ctx, arg)
	}
	return nil
}

type mockTaskDistributor struct {
	distributeWebhookFn func(ctx context.Context, payload SendWebhookPayload, opts ...asynq.Option) error
	distributeTokenFn   func(ctx context.Context, opts ...asynq.Option) error
	distributeIdempFn   func(ctx context.Context, opts ...asynq.Option) error
	distributeScanFn    func(ctx context.Context, opts ...asynq.Option) error
	distributeRemindFn  func(ctx context.Context, payload DeadlineReminderPayload, opts ...asynq.Option) error
	distributeDeleteFn  func(ctx context.Context, payload DeleteStorageObjectPayload, opts ...asynq.Option) error
	closeFn             func() error

	enqueuedWebhooks []SendWebhookPayload
	enqueuedOpts     [][]asynq.Option
	mu               sync.Mutex
}

func (m *mockTaskDistributor) DistributeCleanupExpiredTokens(ctx context.Context, opts ...asynq.Option) error {
	if m.distributeTokenFn != nil {
		return m.distributeTokenFn(ctx, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) DistributeCleanupExpiredIdempotencyKeys(ctx context.Context, opts ...asynq.Option) error {
	if m.distributeIdempFn != nil {
		return m.distributeIdempFn(ctx, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) DistributeScanDueTasks(ctx context.Context, opts ...asynq.Option) error {
	if m.distributeScanFn != nil {
		return m.distributeScanFn(ctx, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) DistributeDeadlineReminder(ctx context.Context, payload DeadlineReminderPayload, opts ...asynq.Option) error {
	if m.distributeRemindFn != nil {
		return m.distributeRemindFn(ctx, payload, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) DistributeSendWebhook(ctx context.Context, payload SendWebhookPayload, opts ...asynq.Option) error {
	m.mu.Lock()
	m.enqueuedWebhooks = append(m.enqueuedWebhooks, payload)
	m.enqueuedOpts = append(m.enqueuedOpts, opts)
	m.mu.Unlock()

	if m.distributeWebhookFn != nil {
		return m.distributeWebhookFn(ctx, payload, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) DistributeDeleteStorageObject(ctx context.Context, payload DeleteStorageObjectPayload, opts ...asynq.Option) error {
	if m.distributeDeleteFn != nil {
		return m.distributeDeleteFn(ctx, payload, opts...)
	}
	return nil
}

func (m *mockTaskDistributor) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func TestOutboxRelay_ProcessBatch(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("EmptyBatchReturnsZero", func(t *testing.T) {
		t.Parallel()
		store := &mockOutboxStore{}
		dist := &mockTaskDistributor{}

		relay := NewOutboxRelay(store, dist, log)
		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("StoreFetchError", func(t *testing.T) {
		t.Parallel()
		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return nil, errors.New("db query timeout")
			},
		}
		dist := &mockTaskDistributor{}

		relay := NewOutboxRelay(store, dist, log)
		count, err := relay.ProcessBatch(context.Background())
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to claim pending outbox events")
	})

	t.Run("EventWithoutSubscribersMarkedPublished", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Write unit tests"}`),
						RetryCount: 0,
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{}, nil
			},
		}
		dist := &mockTaskDistributor{}

		relay := NewOutboxRelay(store, dist, log)
		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Contains(t, store.publishedIDs, eventID)
		assert.Empty(t, dist.enqueuedWebhooks)
	})

	t.Run("EventDispatchedToSubscribersAndPublished", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		webhook1ID := uuid.New()
		webhook2ID := uuid.New()
		fixedCreatedAt := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)

		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 0,
						CreatedAt:  pgtype.Timestamptz{Time: fixedCreatedAt, Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: webhook1ID, Url: "https://example.com/wh1", IsActive: true},
					{ID: webhook2ID, Url: "https://example.com/wh2", IsActive: true},
				}, nil
			},
		}
		dist := &mockTaskDistributor{}

		relay := NewOutboxRelay(store, dist, log)
		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Contains(t, store.publishedIDs, eventID)
		require.Len(t, dist.enqueuedWebhooks, 2)

		expectedDeliveryID1 := uuid.NewSHA1(eventID, webhook1ID[:])
		assert.Equal(t, expectedDeliveryID1, dist.enqueuedWebhooks[0].DeliveryID)

		var env map[string]any
		err = json.Unmarshal(dist.enqueuedWebhooks[0].Payload, &env)
		require.NoError(t, err)
		assert.Equal(t, "task.created", env["event"])
		assert.Equal(t, expectedDeliveryID1.String(), env["id"])
		assert.Equal(t, fixedCreatedAt.Format(time.RFC3339), env["timestamp"])
	})

	t.Run("DeterministicDeliveryID_And_TaskIDConflictTreatedAsSuccess", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		webhookID := uuid.New()
		fixedCreatedAt := time.Date(2026, 11, 28, 9, 30, 0, 0, time.UTC)

		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.updated",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 0,
						CreatedAt:  pgtype.Timestamptz{Time: fixedCreatedAt, Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: webhookID, Url: "https://example.com/wh", IsActive: true},
				}, nil
			},
		}

		callNum := 0
		dist := &mockTaskDistributor{
			distributeWebhookFn: func(_ context.Context, _ SendWebhookPayload, _ ...asynq.Option) error {
				callNum++
				if callNum == 2 {
					// Second processing pass hits TaskIDConflict -> must be treated as success
					return asynq.ErrTaskIDConflict
				}
				return nil
			},
		}

		relay := NewOutboxRelay(store, dist, log)

		// First pass
		count1, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count1)

		// Second pass (duplicate processing)
		count2, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count2)

		require.Len(t, dist.enqueuedWebhooks, 2)
		assert.Equal(t, dist.enqueuedWebhooks[0].DeliveryID, dist.enqueuedWebhooks[1].DeliveryID, "deliveryID must be deterministic across retries")
		assert.NotEmpty(t, dist.enqueuedOpts[0], "asynq.TaskID option must be passed")
		assert.Empty(t, store.retryCalls, "ErrTaskIDConflict must not record a retry")
		assert.Empty(t, store.failedCalls, "ErrTaskIDConflict must not mark failed")
		assert.Len(t, store.publishedIDs, 2)
	})

	t.Run("DistributorFailureRecordsRetry", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		webhookID := uuid.New()

		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 1,
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: webhookID, Url: "https://example.com/wh", IsActive: true},
				}, nil
			},
		}
		dist := &mockTaskDistributor{
			distributeWebhookFn: func(_ context.Context, _ SendWebhookPayload, _ ...asynq.Option) error {
				return errors.New("redis connection refused")
			},
		}

		relay := NewOutboxRelay(store, dist, log, WithMaxRetries(5))
		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Empty(t, store.publishedIDs)
		require.Len(t, store.retryCalls, 1)
		assert.Equal(t, eventID, store.retryCalls[0].ID)
		assert.Contains(t, store.retryCalls[0].LastError.String, "redis connection refused")
	})

	t.Run("MaxRetriesExceededMarksEventFailed", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		webhookID := uuid.New()

		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 4, // Next retry reaches max 5
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: webhookID, Url: "https://example.com/wh", IsActive: true},
				}, nil
			},
		}
		dist := &mockTaskDistributor{
			distributeWebhookFn: func(_ context.Context, _ SendWebhookPayload, _ ...asynq.Option) error {
				return errors.New("permanent failure")
			},
		}

		relay := NewOutboxRelay(store, dist, log, WithMaxRetries(5))
		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Empty(t, store.publishedIDs)
		assert.Empty(t, store.retryCalls)
		require.Len(t, store.failedCalls, 1)
		assert.Equal(t, eventID, store.failedCalls[0].ID)
		assert.Contains(t, store.failedCalls[0].LastError.String, "permanent failure")
	})

	t.Run("PartialFailure_RetriesAndDeduplicatesAlreadyEnqueuedWebhooks", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		wh1 := uuid.New()
		wh2 := uuid.New()
		wh3 := uuid.New()

		// Simulated stateful event status
		eventStatus := "pending"
		var retryCount int32

		store := &mockOutboxStore{
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				if eventStatus != "pending" {
					return nil, nil
				}
				eventStatus = "processing"
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Partial failure test"}`),
						RetryCount: retryCount,
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: wh1, Url: "https://example.com/wh1", IsActive: true},
					{ID: wh2, Url: "https://example.com/wh2", IsActive: true},
					{ID: wh3, Url: "https://example.com/wh3", IsActive: true},
				}, nil
			},
			recordRetryFn: func(_ context.Context, _ repository.RecordOutboxEventRetryParams) error {
				eventStatus = "pending"
				retryCount++
				return nil
			},
			markPublishedFn: func(_ context.Context, _ []uuid.UUID) error {
				eventStatus = "published"
				return nil
			},
		}

		// Simulated Asynq deduplication queue
		enqueuedTaskIDs := make(map[uuid.UUID]int)
		attempt := 0
		dist := &mockTaskDistributor{
			distributeWebhookFn: func(_ context.Context, p SendWebhookPayload, _ ...asynq.Option) error {
				attempt++
				// Fail on wh2 during the first pass (attempt 2)
				if attempt == 2 && p.WebhookID == wh2 {
					return errors.New("temporary redis timeout")
				}
				if enqueuedTaskIDs[p.DeliveryID] > 0 {
					return asynq.ErrTaskIDConflict
				}
				enqueuedTaskIDs[p.DeliveryID]++
				return nil
			},
		}

		relay := NewOutboxRelay(store, dist, log, WithLeaseSeconds(120))

		// Pass 1: wh1 succeeds, wh2 fails -> event returns to pending, retry_count = 1
		count1, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count1)
		assert.Equal(t, "pending", eventStatus)
		assert.Equal(t, int32(1), retryCount)
		assert.Equal(t, []float64{120}, store.reclaimCalls)

		// Pass 2: wh1 hits ErrTaskIDConflict (deduplicated), wh2 & wh3 succeed -> published
		count2, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count2)
		assert.Equal(t, "published", eventStatus)
		assert.Len(t, enqueuedTaskIDs, 3, "each of the 3 webhooks must be effectively enqueued exactly once")
	})

	t.Run("ReclaimStaleLease_ReclaimsAndProcessesOnce", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		whID := uuid.New()

		// Event starts as stale 'processing' (simulating a relay that crashed right after Claim)
		eventStatus := "processing"
		reclaimedOnce := false

		store := &mockOutboxStore{
			reclaimStaleFn: func(_ context.Context, leaseSeconds float64) (int64, error) {
				assert.Equal(t, float64(120), leaseSeconds)
				if eventStatus == "processing" && !reclaimedOnce {
					eventStatus = "pending"
					reclaimedOnce = true
					return 1, nil
				}
				return 0, nil
			},
			claimEventsFn: func(_ context.Context, _ repository.ClaimOutboxEventsParams) ([]repository.ClaimOutboxEventsRow, error) {
				if eventStatus != "pending" {
					return nil, nil
				}
				eventStatus = "processing"
				return []repository.ClaimOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.deleted",
						Payload:    []byte(`{"title":"Crashed relay test"}`),
						RetryCount: 0,
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listActiveWebhooksFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: whID, Url: "https://example.com/wh", IsActive: true},
				}, nil
			},
			markPublishedFn: func(_ context.Context, _ []uuid.UUID) error {
				eventStatus = "published"
				return nil
			},
		}

		dist := &mockTaskDistributor{}
		relay := NewOutboxRelay(store, dist, log, WithLeaseSeconds(120))

		count, err := relay.ProcessBatch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.True(t, reclaimedOnce, "stale processing event must be reclaimed")
		assert.Equal(t, "published", eventStatus)
		assert.Len(t, dist.enqueuedWebhooks, 1)
	})
}

func TestOutboxRelay_LifecycleAndOptions(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	store := &mockOutboxStore{}
	dist := &mockTaskDistributor{}

	relay := NewOutboxRelay(store, dist, log,
		WithPollInterval(10*time.Millisecond),
		WithBatchSize(20),
		WithMaxRetries(3),
	)

	assert.Equal(t, 10*time.Millisecond, relay.pollInterval)
	assert.Equal(t, int32(20), relay.batchSize)
	assert.Equal(t, int32(3), relay.maxRetries)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay.Start(ctx)
	// Give worker time to spin up
	time.Sleep(25 * time.Millisecond)
	relay.Stop()
}
