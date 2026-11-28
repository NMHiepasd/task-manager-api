//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"task-manager-api/internal/repository"
	"task-manager-api/internal/worker"
)

func setupRelayIntegrationDB(t *testing.T) (repository.Store, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_task_manager_relay"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pgContainer.Terminate(context.Background())
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	migrationFiles, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrationFiles)
	sort.Strings(migrationFiles)

	for _, mf := range migrationFiles {
		sqlBytes, readErr := os.ReadFile(mf)
		require.NoError(t, readErr, "failed to read migration %s", mf)
		_, execErr := pool.Exec(ctx, string(sqlBytes))
		require.NoError(t, execErr, "failed to execute migration %s", mf)
	}

	return repository.NewStore(pool), pool
}

// recordingDistributor simulates Asynq TaskID deduplication in-memory with thread safety.
type recordingDistributor struct {
	mu                  sync.Mutex
	seenTaskIDs         map[string]bool
	effectiveDeliveries map[string]int
	conflictCount       int
}

func newRecordingDistributor() *recordingDistributor {
	return &recordingDistributor{
		seenTaskIDs:         make(map[string]bool),
		effectiveDeliveries: make(map[string]int),
	}
}

func (d *recordingDistributor) DistributeCleanupExpiredTokens(context.Context, ...asynq.Option) error {
	return nil
}

func (d *recordingDistributor) DistributeCleanupExpiredIdempotencyKeys(context.Context, ...asynq.Option) error {
	return nil
}

func (d *recordingDistributor) DistributeScanDueTasks(context.Context, ...asynq.Option) error {
	return nil
}

func (d *recordingDistributor) DistributeDeadlineReminder(context.Context, worker.DeadlineReminderPayload, ...asynq.Option) error {
	return nil
}

func (d *recordingDistributor) DistributeDeleteStorageObject(context.Context, worker.DeleteStorageObjectPayload, ...asynq.Option) error {
	return nil
}

func (d *recordingDistributor) Close() error {
	return nil
}

func (d *recordingDistributor) DistributeSendWebhook(_ context.Context, payload worker.SendWebhookPayload, opts ...asynq.Option) error {
	taskID := payload.DeliveryID.String()
	for _, opt := range opts {
		if opt.Type() == asynq.TaskIDOpt {
			if idStr, ok := opt.Value().(string); ok && idStr != "" {
				taskID = idStr
			}
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.seenTaskIDs[taskID] {
		d.conflictCount++
		return asynq.ErrTaskIDConflict
	}

	d.seenTaskIDs[taskID] = true
	d.effectiveDeliveries[taskID]++
	return nil
}

func (d *recordingDistributor) Stats() (totalEffective int, conflicts int, perTask map[string]int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cp := make(map[string]int, len(d.effectiveDeliveries))
	for k, v := range d.effectiveDeliveries {
		cp[k] = v
		totalEffective += v
	}
	return totalEffective, d.conflictCount, cp
}

// crashOnMarkStore wraps repository.Store to simulate a worker crash right after enqueueing and before MarkOutboxEventsPublished.
type crashOnMarkStore struct {
	worker.OutboxStore
}

func (s *crashOnMarkStore) MarkOutboxEventsPublished(_ context.Context, _ []uuid.UUID) error {
	return fmt.Errorf("simulated process crash before marking published")
}

func TestIntegration_OutboxRelay_ConcurrentRelays(t *testing.T) {
	store, pool := setupRelayIntegrationDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Setup user and 2 active webhooks matching "task.created"
	user, err := store.CreateUser(ctx, repository.CreateUserParams{
		Email:        "concurrent_relay@example.com",
		PasswordHash: "hashed_pwd",
		FullName:     "Relay Integration User",
		Role:         "user",
	})
	require.NoError(t, err)

	var webhookIDs [2]uuid.UUID
	for i := 0; i < 2; i++ {
		wh, whErr := store.CreateWebhook(ctx, repository.CreateWebhookParams{
			UserID:    user.ID,
			ProjectID: pgtype.UUID{Valid: false},
			Url:       fmt.Sprintf("https://example.com/wh-%d", i),
			Secret:    "whsec_integration_test_secret_32_bytes_min!",
			Events:    []string{"task.created"},
			IsActive:  true,
		})
		require.NoError(t, whErr)
		webhookIDs[i] = wh.ID
	}

	// 2. Seed 200 outbox events
	const totalEvents = 200
	eventIDs := make([]uuid.UUID, 0, totalEvents)
	payloadBytes, err := json.Marshal(map[string]any{"title": "Concurrent event"})
	require.NoError(t, err)

	for i := 0; i < totalEvents; i++ {
		ev, evErr := store.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.created",
			AggregateType: "task",
			AggregateID:   uuid.New(),
			ActorID:       user.ID,
			ProjectID:     pgtype.UUID{Valid: false},
			Payload:       payloadBytes,
		})
		require.NoError(t, evErr)
		eventIDs = append(eventIDs, ev.ID)
	}

	// 3. Start 3 parallel relays sharing the same PostgreSQL DB and recording distributor
	dist := newRecordingDistributor()
	relays := make([]*worker.OutboxRelay, 3)
	relayCtx, cancelRelays := context.WithCancel(ctx)
	defer cancelRelays()

	for i := 0; i < 3; i++ {
		relays[i] = worker.NewOutboxRelay(
			store,
			dist,
			log,
			worker.WithPollInterval(15*time.Millisecond),
			worker.WithBatchSize(20),
			worker.WithLeaseSeconds(120),
		)
		relays[i].Start(relayCtx)
	}

	// Wait until all 200 events reach 'published' in PostgreSQL
	require.Eventually(t, func() bool {
		var publishedCount int
		qErr := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE status = 'published'").Scan(&publishedCount)
		return qErr == nil && publishedCount == totalEvents
	}, 20*time.Second, 50*time.Millisecond, "all 200 events must be published")

	for _, r := range relays {
		r.Stop()
	}

	// 4. Verify every (event, webhook) pair was effectively enqueued exactly once (200 * 2 = 400)
	totalEffective, _, perTask := dist.Stats()
	assert.Equal(t, totalEvents*2, totalEffective, "total effective deliveries must equal events * webhooks")
	assert.Len(t, perTask, totalEvents*2)

	for _, evID := range eventIDs {
		for _, whID := range webhookIDs {
			expectedDeliveryID := uuid.NewSHA1(evID, whID[:]).String()
			assert.Equal(t, 1, perTask[expectedDeliveryID], "pair (%s, %s) must be enqueued exactly once", evID, whID)
		}
	}
}

func TestIntegration_OutboxRelay_CrashBetweenEnqueueAndMarkPublished(t *testing.T) {
	store, pool := setupRelayIntegrationDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Setup user and 2 active webhooks matching "task.updated"
	user, err := store.CreateUser(ctx, repository.CreateUserParams{
		Email:        "crash_recovery@example.com",
		PasswordHash: "hashed_pwd",
		FullName:     "Crash Recovery User",
		Role:         "user",
	})
	require.NoError(t, err)

	var webhookIDs [2]uuid.UUID
	for i := 0; i < 2; i++ {
		wh, whErr := store.CreateWebhook(ctx, repository.CreateWebhookParams{
			UserID:    user.ID,
			ProjectID: pgtype.UUID{Valid: false},
			Url:       fmt.Sprintf("https://example.com/crash-wh-%d", i),
			Secret:    "whsec_integration_test_secret_32_bytes_min!",
			Events:    []string{"task.updated"},
			IsActive:  true,
		})
		require.NoError(t, whErr)
		webhookIDs[i] = wh.ID
	}

	// 2. Seed 5 outbox events
	const totalEvents = 5
	eventIDs := make([]uuid.UUID, 0, totalEvents)
	payloadBytes, err := json.Marshal(map[string]any{"title": "Crash recovery event"})
	require.NoError(t, err)

	for i := 0; i < totalEvents; i++ {
		ev, evErr := store.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.updated",
			AggregateType: "task",
			AggregateID:   uuid.New(),
			ActorID:       user.ID,
			ProjectID:     pgtype.UUID{Valid: false},
			Payload:       payloadBytes,
		})
		require.NoError(t, evErr)
		eventIDs = append(eventIDs, ev.ID)
	}

	dist := newRecordingDistributor()

	// 3. Relay 1 claims, enqueues all 10 tasks, then "crashes" before MarkOutboxEventsPublished succeeds
	crashingRelay := worker.NewOutboxRelay(
		&crashOnMarkStore{OutboxStore: store},
		dist,
		log,
		worker.WithBatchSize(10),
		worker.WithLeaseSeconds(1),
	)
	_, err = crashingRelay.ProcessBatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated process crash")

	// Confirm all 5 events are stuck in 'processing' in PostgreSQL
	var processingCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE status = 'processing'").Scan(&processingCount)
	require.NoError(t, err)
	assert.Equal(t, totalEvents, processingCount)

	// 4. Wait for the 1-second lease to expire, then Relay 2 reclaims and processes
	time.Sleep(1100 * time.Millisecond)

	healthyRelay := worker.NewOutboxRelay(
		store,
		dist,
		log,
		worker.WithBatchSize(10),
		worker.WithLeaseSeconds(1),
	)
	publishedCount, err := healthyRelay.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, totalEvents, publishedCount)

	// 5. Verify DB state is 'published' and deduplication kept total effective tasks at exactly 10
	var dbPublished int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE status = 'published'").Scan(&dbPublished)
	require.NoError(t, err)
	assert.Equal(t, totalEvents, dbPublished)

	totalEffective, conflicts, perTask := dist.Stats()
	assert.Equal(t, totalEvents*2, totalEffective, "each (event, webhook) pair must only have 1 effective task")
	assert.Equal(t, totalEvents*2, conflicts, "relay 2 re-enqueue must hit TaskIDConflict for all 10 tasks")
	for _, evID := range eventIDs {
		for _, whID := range webhookIDs {
			expectedDeliveryID := uuid.NewSHA1(evID, whID[:]).String()
			assert.Equal(t, 1, perTask[expectedDeliveryID])
		}
	}
}
