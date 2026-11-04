package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Setup initializes the global slog default logger with a JSON handler and specified level.
func Setup(levelStr, env string) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if strings.ToLower(env) == "development" {
		opts.AddSource = true
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	l := slog.New(handler)
	slog.SetDefault(l)

	return l
}
