package worker

import (
	"fmt"
	"log/slog"
	"task-manager-api/internal/config"

	"github.com/hibiken/asynq"
)

// Scheduler manages recurring background tasks using Asynq.
type Scheduler struct {
	scheduler *asynq.Scheduler
	log       *slog.Logger
}

// NewScheduler creates and configures a periodic task scheduler with recurring jobs.
func NewScheduler(redisOpt asynq.RedisConnOpt, cfg *config.Config, log *slog.Logger) (*Scheduler, error) {
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{})

	// 1. Register expired refresh tokens cleanup job
	cleanupSchedule := cfg.CleanupCronSchedule
	if cleanupSchedule == "" {
		cleanupSchedule = "@every 6h"
	}
	cleanupTask := NewCleanupExpiredTokensTask()
	entryID, err := scheduler.Register(cleanupSchedule, cleanupTask)
	if err != nil {
		return nil, fmt.Errorf("failed to register cleanup expired tokens cron: %w", err)
	}
	log.Info("Registered recurring cleanup job", "entry_id", entryID, "schedule", cleanupSchedule)

	// 2. Register upcoming task deadline scanner job
	scanSchedule := cfg.DeadlineReminderCronSchedule
	if scanSchedule == "" {
		scanSchedule = "@hourly"
	}
	scanTask := NewScanDueTasksTask()
	entryID, err = scheduler.Register(scanSchedule, scanTask)
	if err != nil {
		return nil, fmt.Errorf("failed to register deadline scanner cron: %w", err)
	}
	log.Info("Registered recurring deadline scanner job", "entry_id", entryID, "schedule", scanSchedule)

	return &Scheduler{
		scheduler: scheduler,
		log:       log,
	}, nil
}

// Start launches the scheduler in the background.
func (s *Scheduler) Start() error {
	s.log.Info("Starting background cron scheduler")
	return s.scheduler.Start()
}

// Shutdown gracefully terminates the recurring task scheduler.
func (s *Scheduler) Shutdown() {
	s.log.Info("Shutting down background cron scheduler")
	s.scheduler.Shutdown()
}
