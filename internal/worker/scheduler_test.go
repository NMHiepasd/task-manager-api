package worker

import (
	"io"
	"log/slog"
	"task-manager-api/internal/config"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRedisURI(t *testing.T) {
	opt, err := asynq.ParseRedisURI("redis://localhost:6379/0")
	require.NoError(t, err)
	assert.NotNil(t, opt)
}

func TestNewScheduler_InvalidSchedule(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	cfg := &config.Config{
		CleanupCronSchedule: "invalid-cron-format",
	}

	redisOpt := asynq.RedisClientOpt{Addr: "localhost:6379"}
	scheduler, err := NewScheduler(redisOpt, cfg, log)
	assert.Error(t, err)
	assert.Nil(t, scheduler)
	assert.Contains(t, err.Error(), "failed to register cleanup expired tokens cron")
}
