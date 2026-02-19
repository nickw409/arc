package orchestrator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOrchestratorLockAcquireRelease(t *testing.T) {
	planDir := t.TempDir()

	// Acquire lock
	if err := acquireLock(planDir); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Verify .orchestrator.lock exists with PID
	lockPath := filepath.Join(planDir, ".orchestrator.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file not found: %v", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("lock file doesn't contain valid PID: %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("lock file PID=%d, want %d", pid, os.Getpid())
	}

	// Release lock
	releaseLock(planDir)

	// Verify lock is gone
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("expected lock file to be removed after release")
	}
}

func TestOrchestratorLockAlreadyHeld(t *testing.T) {
	planDir := t.TempDir()

	// Acquire lock first time
	if err := acquireLock(planDir); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer releaseLock(planDir)

	// Attempt second acquire — should fail
	err := acquireLock(planDir)
	if err == nil {
		t.Fatal("expected error for already-held lock")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already running") {
		t.Fatalf("expected error containing 'already running', got: %v", err)
	}
}

func TestOrchestratorLockStale(t *testing.T) {
	planDir := t.TempDir()

	// Write lock file with dead PID (999999 is almost certainly not alive)
	lockPath := filepath.Join(planDir, ".orchestrator.lock")
	if err := os.WriteFile(lockPath, []byte("999999"), 0644); err != nil {
		t.Fatal(err)
	}

	// Acquire should detect stale lock and succeed
	if err := acquireLock(planDir); err != nil {
		t.Fatalf("expected stale lock to be cleaned up, got error: %v", err)
	}
	defer releaseLock(planDir)

	// Verify lock now has our PID
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid != os.Getpid() {
		t.Fatalf("lock file PID=%d, want %d", pid, os.Getpid())
	}
}

func TestOrchestratorLockReleaseMissing(t *testing.T) {
	planDir := t.TempDir()

	// Release when no lock file exists — should not error (idempotent)
	releaseLock(planDir)
	// If we got here without panic, the test passes
}
