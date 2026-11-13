package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"task-manager-api/internal/repository"
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
	deleteExpiredTokensFn func(ctx context.Context) error
	listTasksDueSoonFn    func(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error)
}

func (m *mockWorkerStore) DeleteExpiredRefreshTokens(ctx context.Context) error {
	if m.deleteExpiredTokensFn != nil {
		return m.deleteExpiredTokensFn(ctx)
	}
	return nil
}

func (m *mockWorkerStore) ListTasksDueSoon(ctx context.Context, arg repository.ListTasksDueSoonParams) ([]repository.ListTasksDueSoonRow, error) {
	if m.listTasksDueSoonFn != nil {
		return m.listTasksDueSoonFn(ctx, arg)
	}
	return nil, nil
}

// mockDistributor mocks TaskDistributor
type mockDistributor struct {
	reminders []DeadlineReminderPayload
}

func (m *mockDistributor) DistributeCleanupExpiredTokens(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockDistributor) DistributeScanDueTasks(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockDistributor) DistributeDeadlineReminder(_ context.Context, payload DeadlineReminderPayload, _ ...asynq.Option) error {
	m.reminders = append(m.reminders, payload)
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
