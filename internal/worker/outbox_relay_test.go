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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOutboxStore struct {
	mu sync.Mutex

	fetchPendingFn       func(ctx context.Context, arg repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error)
	listActiveWebhooksFn func(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error)
	markPublishedFn      func(ctx context.Context, ids []uuid.UUID) error
	recordRetryFn        func(ctx context.Context, arg repository.RecordOutboxEventRetryParams) error
	markFailedFn         func(ctx context.Context, arg repository.MarkOutboxEventFailedParams) error

	publishedIDs []uuid.UUID
	retryCalls   []repository.RecordOutboxEventRetryParams
	failedCalls  []repository.MarkOutboxEventFailedParams
}

func (m *mockOutboxStore) FetchPendingOutboxEvents(ctx context.Context, arg repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
	if m.fetchPendingFn != nil {
		return m.fetchPendingFn(ctx, arg)
	}
	return nil, nil
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
			fetchPendingFn: func(_ context.Context, _ repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
				return nil, errors.New("db query timeout")
			},
		}
		dist := &mockTaskDistributor{}

		relay := NewOutboxRelay(store, dist, log)
		count, err := relay.ProcessBatch(context.Background())
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to fetch pending outbox events")
	})

	t.Run("EventWithoutSubscribersMarkedPublished", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		store := &mockOutboxStore{
			fetchPendingFn: func(_ context.Context, _ repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
				return []repository.FetchPendingOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Write unit tests"}`),
						RetryCount: 0,
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

		store := &mockOutboxStore{
			fetchPendingFn: func(_ context.Context, _ repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
				return []repository.FetchPendingOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 0,
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

		var env map[string]any
		err = json.Unmarshal(dist.enqueuedWebhooks[0].Payload, &env)
		require.NoError(t, err)
		assert.Equal(t, "task.created", env["event"])
		assert.NotEmpty(t, env["id"])
		assert.NotEmpty(t, env["timestamp"])
	})

	t.Run("DistributorFailureRecordsRetry", func(t *testing.T) {
		t.Parallel()
		eventID := uuid.New()
		webhookID := uuid.New()

		store := &mockOutboxStore{
			fetchPendingFn: func(_ context.Context, _ repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
				return []repository.FetchPendingOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 1,
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
			fetchPendingFn: func(_ context.Context, _ repository.FetchPendingOutboxEventsParams) ([]repository.FetchPendingOutboxEventsRow, error) {
				return []repository.FetchPendingOutboxEventsRow{
					{
						ID:         eventID,
						EventType:  "task.created",
						Payload:    []byte(`{"title":"Task 1"}`),
						RetryCount: 4, // Next retry reaches max 5
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
