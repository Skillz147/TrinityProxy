package logutil

import (
	"log/slog"
	"os"
)

// New creates a structured logger for the given component and sets it as slog default.
func New(component string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	if component != "" {
		return logger.With("component", component)
	}
	return logger
}

// Component returns a logger with the given component field using the current default.
func Component(name string) *slog.Logger {
	return slog.Default().With("component", name)
}

// Fatal logs at error level and exits with code 1.
func Fatal(log *slog.Logger, msg string, args ...any) {
	log.Error(msg, args...)
	os.Exit(1)
}
