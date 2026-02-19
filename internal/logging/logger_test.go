package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileLoggerWritesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	logger, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer Close()

	logger.Info("test msg", "key", "value")
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing JSON: %v (data: %s)", err, data)
	}

	if entry["msg"] != "test msg" {
		t.Errorf("msg=%v, want 'test msg'", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("key=%v, want 'value'", entry["key"])
	}
}

func TestFileLoggerCloseFlushes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	logger, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("flush test")
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("log file empty after close")
	}
}

func TestFileLoggerCloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	_, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestFileLoggerEmptyPath(t *testing.T) {
	_, err := NewFileLogger("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestFileLoggerPermissionError(t *testing.T) {
	_, err := NewFileLogger("/root/noperm.log")
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

func TestCloseWithoutNewLogger(t *testing.T) {
	// Reset state
	mu.Lock()
	logFile = nil
	mu.Unlock()

	if err := Close(); err != nil {
		t.Fatalf("Close without logger: %v", err)
	}
}

func TestFileLoggerAlreadyExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	os.WriteFile(path, []byte("old content"), 0644)

	logger, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("new content")
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should not contain old content (file was truncated)
	if string(data[:3]) == "old" {
		t.Fatal("file was not truncated")
	}
}
