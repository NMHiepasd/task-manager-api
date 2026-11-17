package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/storage"
	"task-manager-api/pkg/webhook"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWorkerStore mocks TaskStore for background worker tests
type mockWorkerStore struct {
	deleteExpiredTokensFn      func(ctx context.Context) error
	deleteOldIdempotencyKeysFn func(ctx context.Context, cutoff pgtype.Timestamptz) error
	listTasksDueSoonFn         func(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error)
	createWebhookDeliveryFn    func(ctx context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error)
}

func (m *mockWorkerStore) DeleteExpiredRefreshTokens(ctx context.Context) error {
	if m.deleteExpiredTokensFn != nil {
		return m.deleteExpiredTokensFn(ctx)
	}
	return nil
}

func (m *mockWorkerStore) DeleteOldIdempotencyKeys(ctx context.Context, cutoff pgtype.Timestamptz) error {
	if m.deleteOldIdempotencyKeysFn != nil {
		return m.deleteOldIdempotencyKeysFn(ctx, cutoff)
	}
	return nil
}

func (m *mockWorkerStore) ListTasksDueSoon(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error) {
	if m.listTasksDueSoonFn != nil {
		return m.listTasksDueSoonFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockWorkerStore) CreateWebhookDelivery(ctx context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
	if m.createWebhookDeliveryFn != nil {
		return m.createWebhookDeliveryFn(ctx, arg)
	}
	return repository.WebhookDelivery{}, nil
}

type errTransport struct {
	err error
}

func (t *errTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, t.err
}

// mockDistributor mocks TaskDistributor
type mockDistributor struct {
	reminders      []DeadlineReminderPayload
	webhooks       []SendWebhookPayload
	storageDeletes []DeleteStorageObjectPayload
}

func (m *mockDistributor) DistributeCleanupExpiredTokens(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockDistributor) DistributeCleanupExpiredIdempotencyKeys(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockDistributor) DistributeScanDueTasks(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockDistributor) DistributeDeadlineReminder(_ context.Context, payload DeadlineReminderPayload, _ ...asynq.Option) error {
	m.reminders = append(m.reminders, payload)
	return nil
}

func (m *mockDistributor) DistributeSendWebhook(_ context.Context, payload SendWebhookPayload, _ ...asynq.Option) error {
	m.webhooks = append(m.webhooks, payload)
	return nil
}

func (m *mockDistributor) DistributeDeleteStorageObject(_ context.Context, payload DeleteStorageObjectPayload, _ ...asynq.Option) error {
	m.storageDeletes = append(m.storageDeletes, payload)
	return nil
}

func (m *mockDistributor) Close() error { return nil }

func TestProcessCleanupExpiredTokens(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("success", func(t *testing.T) {
		called := false
		store := &mockWorkerStore{
			deleteExpiredTokensFn: func(_ context.Context) error {
				called = true
				return nil
			},
		}

		processor := &RedisTaskProcessor{
			store: store,
			log:   log,
		}

		task := asynq.NewTask(TypeCleanupExpiredTokens, nil)
		err := processor.ProcessCleanupExpiredTokens(context.Background(), task)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("database error", func(t *testing.T) {
		store := &mockWorkerStore{
			deleteExpiredTokensFn: func(_ context.Context) error {
				return errors.New("connection failed")
			},
		}

		processor := &RedisTaskProcessor{
			store: store,
			log:   log,
		}

		task := asynq.NewTask(TypeCleanupExpiredTokens, nil)
		err := processor.ProcessCleanupExpiredTokens(context.Background(), task)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection failed")
	})
}

func TestProcessCleanupExpiredIdempotencyKeys(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("success", func(t *testing.T) {
		called := false
		store := &mockWorkerStore{
			deleteOldIdempotencyKeysFn: func(_ context.Context, cutoff pgtype.Timestamptz) error {
				called = true
				assert.True(t, cutoff.Valid)
				assert.True(t, cutoff.Time.Before(time.Now()))
				return nil
			},
		}

		processor := &RedisTaskProcessor{
			store: store,
			log:   log,
		}

		task := asynq.NewTask(TypeCleanupExpiredIdempotencyKeys, nil)
		err := processor.ProcessCleanupExpiredIdempotencyKeys(context.Background(), task)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("database error", func(t *testing.T) {
		store := &mockWorkerStore{
			deleteOldIdempotencyKeysFn: func(_ context.Context, _ pgtype.Timestamptz) error {
				return errors.New("delete failed")
			},
		}

		processor := &RedisTaskProcessor{
			store: store,
			log:   log,
		}

		task := asynq.NewTask(TypeCleanupExpiredIdempotencyKeys, nil)
		err := processor.ProcessCleanupExpiredIdempotencyKeys(context.Background(), task)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delete failed")
	})
}

func TestProcessScanDueTasks(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	taskID := uuid.New()
	due := time.Now().Add(5 * time.Hour)

	store := &mockWorkerStore{
		listTasksDueSoonFn: func(_ context.Context, _ repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error) {
			return []repository.ListTasksDueSoonRow{
				{
					ID:             taskID,
					Title:          "Urgent Report",
					Status:         "todo",
					DueDate:        pgtype.Timestamptz{Time: due, Valid: true},
					RecipientEmail: "admin@example.com",
					RecipientName:  "Admin User",
				},
			}, nil
		},
	}

	dist := &mockDistributor{}
	processor := &RedisTaskProcessor{
		store:       store,
		distributor: dist,
		log:         log,
	}

	task := asynq.NewTask(TypeScanDueTasks, nil)
	err := processor.ProcessScanDueTasks(context.Background(), task)
	require.NoError(t, err)

	require.Len(t, dist.reminders, 1)
	assert.Equal(t, taskID, dist.reminders[0].TaskID)
	assert.Equal(t, "admin@example.com", dist.reminders[0].RecipientEmail)
	assert.Equal(t, "Urgent Report", dist.reminders[0].TaskTitle)
}

func TestProcessDeadlineReminder(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	processor := &RedisTaskProcessor{
		log: log,
	}

	t.Run("valid payload", func(t *testing.T) {
		payload := DeadlineReminderPayload{
			TaskID:         uuid.New(),
			RecipientEmail: "lead@example.com",
			RecipientName:  "Tech Lead",
			TaskTitle:      "Fix Critical Vulnerability",
			DueDate:        time.Now().Add(1 * time.Hour),
		}

		data, err := json.Marshal(payload)
		require.NoError(t, err)

		task := asynq.NewTask(TypeDeadlineReminder, data)
		err = processor.ProcessDeadlineReminder(context.Background(), task)
		assert.NoError(t, err)
	})

	t.Run("invalid json payload returns SkipRetry", func(t *testing.T) {
		task := asynq.NewTask(TypeDeadlineReminder, []byte("invalid-json"))
		err := processor.ProcessDeadlineReminder(context.Background(), task)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, asynq.SkipRetry))
	})
}

func TestProcessSendWebhook(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("success delivery with valid HMAC V2", func(t *testing.T) {
		var receivedEvent, receivedSig, receivedID, receivedTs string
		var receivedBody []byte

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedEvent = r.Header.Get("X-Webhook-Event")
			receivedSig = r.Header.Get("X-Webhook-Signature")
			receivedID = r.Header.Get("X-Webhook-Id")
			receivedTs = r.Header.Get("X-Webhook-Timestamp")
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"received":true}`))
		}))
		defer ts.Close()

		var recordedDelivery repository.CreateWebhookDeliveryParams
		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				recordedDelivery = arg
				return repository.WebhookDelivery{}, nil
			},
		}

		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: ts.Client(),
		}

		deliveryID := uuid.New()
		webhookID := uuid.New()
		//nolint:gosec // G101: Test dummy secret key
		secret := "whsec_test_secret_32_characters_minimum_len!"
		payloadBody := []byte(`{"task_id":"123","title":"Hello"}`)

		payload := SendWebhookPayload{
			DeliveryID: deliveryID,
			WebhookID:  webhookID,
			TargetURL:  ts.URL,
			Secret:     secret,
			EventType:  "task.created",
			Payload:    payloadBody,
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		err = processor.ProcessSendWebhook(context.Background(), task)
		assert.NoError(t, err)

		assert.Equal(t, "task.created", receivedEvent)
		assert.Equal(t, payloadBody, receivedBody)
		assert.Equal(t, deliveryID.String(), receivedID)
		assert.NotEmpty(t, receivedTs)
		assert.True(t, strings.HasPrefix(receivedSig, "v1="))

		// Receiver verifies authenticity using VerifyV2
		verifyErr := webhook.VerifyV2(secret, receivedTs, receivedID, receivedBody, receivedSig, time.Now(), 5*time.Minute)
		assert.NoError(t, verifyErr, "receiver must verify signature using VerifyV2")

		assert.Equal(t, "success", recordedDelivery.Status)
		assert.Equal(t, int32(200), recordedDelivery.ResponseStatus.Int32)
	})

	t.Run("retry preserves delivery_id with new timestamp and valid V2 signature", func(t *testing.T) {
		var timestamps []string
		var ids []string
		var sigs []string
		var bodies [][]byte

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timestamps = append(timestamps, r.Header.Get("X-Webhook-Timestamp"))
			ids = append(ids, r.Header.Get("X-Webhook-Id"))
			sigs = append(sigs, r.Header.Get("X-Webhook-Signature"))
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, b)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, _ repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				return repository.WebhookDelivery{}, nil
			},
		}

		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: ts.Client(),
		}

		deliveryID := uuid.New()
		//nolint:gosec // G101: Test dummy secret key
		secret := "whsec_test_secret_32_characters_minimum_len!"
		payloadBody := []byte(`{"id":"task-retry-test"}`)

		payload := SendWebhookPayload{
			DeliveryID: deliveryID,
			WebhookID:  uuid.New(),
			TargetURL:  ts.URL,
			Secret:     secret,
			EventType:  "task.updated",
			Payload:    payloadBody,
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		// First delivery attempt
		err = processor.ProcessSendWebhook(context.Background(), task)
		require.NoError(t, err)

		// Simulate passage of time for retry (sleep 1 second to advance Unix timestamp)
		time.Sleep(1100 * time.Millisecond)

		// Second delivery attempt (retry of the exact same task)
		err = processor.ProcessSendWebhook(context.Background(), task)
		require.NoError(t, err)

		require.Len(t, ids, 2)
		require.Len(t, timestamps, 2)

		// ID must remain stable across retries
		assert.Equal(t, deliveryID.String(), ids[0])
		assert.Equal(t, deliveryID.String(), ids[1])
		assert.Equal(t, ids[0], ids[1], "Delivery ID must be preserved across retries")

		// Timestamp must be updated on retry
		assert.NotEqual(t, timestamps[0], timestamps[1], "Retry must generate a fresh timestamp")

		// Both attempts must be verifiable using VerifyV2
		err1 := webhook.VerifyV2(secret, timestamps[0], ids[0], bodies[0], sigs[0], time.Now(), 5*time.Minute)
		assert.NoError(t, err1, "first attempt must verify with VerifyV2")

		err2 := webhook.VerifyV2(secret, timestamps[1], ids[1], bodies[1], sigs[1], time.Now(), 5*time.Minute)
		assert.NoError(t, err2, "second attempt must verify with VerifyV2")
	})

	t.Run("receiver server error 500 returns error for retry", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server crashed"))
		}))
		defer ts.Close()

		var recordedDelivery repository.CreateWebhookDeliveryParams
		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				recordedDelivery = arg
				return repository.WebhookDelivery{}, nil
			},
		}

		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: ts.Client(),
		}

		payload := SendWebhookPayload{
			WebhookID: uuid.New(),
			TargetURL: ts.URL,
			Secret:    "secret",
			EventType: "task.deleted",
			Payload:   []byte(`{"task_id":"123"}`),
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		err = processor.ProcessSendWebhook(context.Background(), task)
		assert.Error(t, err)
		assert.Equal(t, "failed", recordedDelivery.Status)
		assert.Equal(t, int32(500), recordedDelivery.ResponseStatus.Int32)
	})

	t.Run("invalid json payload returns SkipRetry", func(t *testing.T) {
		processor := &RedisTaskProcessor{
			log: log,
		}

		task := asynq.NewTask(TypeSendWebhook, []byte("invalid-json"))
		err := processor.ProcessSendWebhook(context.Background(), task)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, asynq.SkipRetry))
	})

	t.Run("ssrf blocked returns SkipRetry", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		var recordedDelivery repository.CreateWebhookDeliveryParams
		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				recordedDelivery = arg
				return repository.WebhookDelivery{}, nil
			},
		}

		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: webhook.NewSafeHTTPClient(false, 2*time.Second),
		}

		payload := SendWebhookPayload{
			WebhookID: uuid.New(),
			TargetURL: ts.URL, // ts.URL is 127.0.0.1, must be blocked
			Secret:    "secret",
			EventType: "task.created",
			Payload:   []byte(`{"task_id":"123"}`),
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		err = processor.ProcessSendWebhook(context.Background(), task)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, asynq.SkipRetry))
		assert.Equal(t, "failed", recordedDelivery.Status)
		assert.Contains(t, recordedDelivery.ResponseBody.String, "SSRF prevention")
	})

	t.Run("normal network error does not return SkipRetry", func(t *testing.T) {
		var recordedDelivery repository.CreateWebhookDeliveryParams
		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				recordedDelivery = arg
				return repository.WebhookDelivery{}, nil
			},
		}

		mockTransport := &errTransport{err: errors.New("dial tcp 93.184.216.34:443: i/o timeout")}
		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: &http.Client{Transport: mockTransport},
		}

		payload := SendWebhookPayload{
			WebhookID: uuid.New(),
			TargetURL: "https://example.com/webhook",
			Secret:    "secret",
			EventType: "task.created",
			Payload:   []byte(`{"task_id":"123"}`),
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		err = processor.ProcessSendWebhook(context.Background(), task)
		assert.Error(t, err)
		assert.False(t, errors.Is(err, asynq.SkipRetry), "generic network error should be retryable (not SkipRetry)")
		assert.Equal(t, "failed", recordedDelivery.Status)
	})

	t.Run("redirect response is treated as failed and not followed", func(t *testing.T) {
		redirectTargetCalled := false
		targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			redirectTargetCalled = true
			w.WriteHeader(http.StatusOK)
		}))
		defer targetServer.Close()

		redirectingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, targetServer.URL, http.StatusFound)
		}))
		defer redirectingServer.Close()

		var recordedDelivery repository.CreateWebhookDeliveryParams
		store := &mockWorkerStore{
			createWebhookDeliveryFn: func(_ context.Context, arg repository.CreateWebhookDeliveryParams) (repository.WebhookDelivery, error) {
				recordedDelivery = arg
				return repository.WebhookDelivery{}, nil
			},
		}

		processor := &RedisTaskProcessor{
			store:      store,
			log:        log,
			httpClient: webhook.NewSafeHTTPClient(true, 2*time.Second),
		}

		payload := SendWebhookPayload{
			WebhookID: uuid.New(),
			TargetURL: redirectingServer.URL,
			Secret:    "secret",
			EventType: "task.updated",
			Payload:   []byte(`{"task_id":"123"}`),
		}

		task, err := NewSendWebhookTask(payload)
		require.NoError(t, err)

		err = processor.ProcessSendWebhook(context.Background(), task)
		assert.NoError(t, err, "3xx redirect is a terminal delivery failure and does not trigger retry")
		assert.False(t, redirectTargetCalled, "should not have followed redirect")
		assert.Equal(t, "failed", recordedDelivery.Status)
		assert.Equal(t, int32(302), recordedDelivery.ResponseStatus.Int32)
	})
}

type mockWorkerStorageClient struct {
	deleteFn func(ctx context.Context, key string) error
}

func (m *mockWorkerStorageClient) Upload(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (m *mockWorkerStorageClient) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockWorkerStorageClient) Delete(ctx context.Context, key string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, key)
	}
	return nil
}

func (m *mockWorkerStorageClient) GetPresignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (m *mockWorkerStorageClient) EnsureBucket(context.Context) error {
	return nil
}

func TestProcessDeleteStorageObject(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("Success", func(t *testing.T) {
		deletedKey := ""
		store := &mockWorkerStorageClient{
			deleteFn: func(_ context.Context, key string) error {
				deletedKey = key
				return nil
			},
		}

		processor := &RedisTaskProcessor{
			storage: store,
			log:     log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "tasks/123/file.pdf",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.NoError(t, err)
		assert.Equal(t, "tasks/123/file.pdf", deletedKey)
	})

	t.Run("RetryOnTransientError", func(t *testing.T) {
		store := &mockWorkerStorageClient{
			deleteFn: func(_ context.Context, _ string) error {
				return errors.New("connection reset by peer")
			},
		}

		processor := &RedisTaskProcessor{
			storage: store,
			log:     log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "tasks/123/file.pdf",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.Error(t, err)
		assert.False(t, errors.Is(err, asynq.SkipRetry), "transient error must trigger Asynq retry")
	})

	t.Run("SkipRetryWhenObjectNotFound", func(t *testing.T) {
		store := &mockWorkerStorageClient{
			deleteFn: func(_ context.Context, _ string) error {
				return storage.ErrObjectNotFound
			},
		}

		processor := &RedisTaskProcessor{
			storage: store,
			log:     log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "tasks/123/file.pdf",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.NoError(t, err, "non-existent object must be skipped and considered done")
	})

	t.Run("SkipRetryWhenMinIONoSuchKeyError", func(t *testing.T) {
		store := &mockWorkerStorageClient{
			deleteFn: func(_ context.Context, _ string) error {
				return errors.New("The specified key does not exist. (NoSuchKey)")
			},
		}

		processor := &RedisTaskProcessor{
			storage: store,
			log:     log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "tasks/123/file.pdf",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.NoError(t, err, "NoSuchKey error must be skipped and considered done")
	})

	t.Run("SkipRetryOnInvalidPayloadJSON", func(t *testing.T) {
		processor := &RedisTaskProcessor{
			log: log,
		}

		task := asynq.NewTask(TypeDeleteStorageObject, []byte("invalid-json"))
		err := processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, asynq.SkipRetry), "corrupt payload must not be retried")
	})

	t.Run("SkipOnEmptyStorageKey", func(t *testing.T) {
		processor := &RedisTaskProcessor{
			log: log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.NoError(t, err)
	})

	t.Run("ErrorWhenStorageNotConfigured", func(t *testing.T) {
		processor := &RedisTaskProcessor{
			storage: nil,
			log:     log,
		}

		task, err := NewDeleteStorageObjectTask(DeleteStorageObjectPayload{
			StorageKey: "tasks/123/file.pdf",
		})
		require.NoError(t, err)

		err = processor.ProcessDeleteStorageObject(context.Background(), task)
		assert.Error(t, err)
	})
}
