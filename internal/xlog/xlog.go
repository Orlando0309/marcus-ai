package xlog

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the default slog logger from MARCUS_LOG (debug, info, warn, error).
func Init() {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MARCUS_LOG"))) {
	case "debug", "1", "true", "yes":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}
