package logging

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var (
	logFile *os.File
	mu      sync.Mutex
)

// NewFileLogger creates a slog.Logger that writes structured JSON to a file.
// Use when the TUI is active so log output goes to file instead of stderr.
func NewFileLogger(path string) (*slog.Logger, error) {
	if path == "" {
		return nil, fmt.Errorf("log file path cannot be empty")
	}

	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	logFile = f
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	return slog.New(handler), nil
}

// Close flushes and closes the log file. Safe to call multiple times.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return nil
	}

	err := logFile.Close()
	logFile = nil
	return err
}
