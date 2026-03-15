package state

import (
	"testing"
)

func TestSetStatus(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := SetStatus(sf, "implementing"); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus != "implementing" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "implementing")
	}
}

func TestSetStatusEmpty(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := SetStatus(sf, ""); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus != "" {
		t.Fatalf("PhaseStatus = %q, want empty", got.PhaseStatus)
	}
}

func TestUpdateTests(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := UpdateTests(sf, 5, 10); err != nil {
		t.Fatalf("UpdateTests error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.TestsPassing != 5 {
		t.Fatalf("TestsPassing = %d, want 5", got.TestsPassing)
	}
	if got.TestsTotal != 10 {
		t.Fatalf("TestsTotal = %d, want 10", got.TestsTotal)
	}
}

func TestUpdateTestsZeroTotal(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := UpdateTests(sf, 0, 0); err != nil {
		t.Fatalf("UpdateTests error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.TestsPassing != 0 {
		t.Fatalf("TestsPassing = %d, want 0", got.TestsPassing)
	}
	if got.TestsTotal != 0 {
		t.Fatalf("TestsTotal = %d, want 0", got.TestsTotal)
	}
}

func TestIncrementIteration(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := IncrementIteration(sf); err != nil {
		t.Fatalf("IncrementIteration error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.Iteration.Current != 1 {
		t.Fatalf("Iteration.Current = %d, want 1", got.Iteration.Current)
	}
}

func TestAddTestFile(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.TestFiles) != 1 {
		t.Fatalf("len(TestFiles) = %d, want 1", len(got.TestFiles))
	}
	if got.TestFiles[0] != "tests/test_core.go" {
		t.Fatalf("TestFiles[0] = %q, want %q", got.TestFiles[0], "tests/test_core.go")
	}
}

func TestAddTestFileDedup(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}
	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.TestFiles) != 1 {
		t.Fatalf("len(TestFiles) = %d, want 1 (no duplicate)", len(got.TestFiles))
	}
}

func TestAddTestFileEmptyPath(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, ""); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	found := false
	for _, f := range got.TestFiles {
		if f == "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("TestFiles should contain empty string")
	}
}

// containsStr is a simple helper to avoid importing strings in this file.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestIncrementWatchAttempts(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	// First increment: 0 → 1
	if err := IncrementWatchAttempts(sf); err != nil {
		t.Fatalf("IncrementWatchAttempts (1): %v", err)
	}
	s, _ := sf.Read()
	if s.WatchAttempts != 1 {
		t.Errorf("WatchAttempts after 1st: got %d, want 1", s.WatchAttempts)
	}

	// Second increment: 1 → 2
	if err := IncrementWatchAttempts(sf); err != nil {
		t.Fatalf("IncrementWatchAttempts (2): %v", err)
	}
	s, _ = sf.Read()
	if s.WatchAttempts != 2 {
		t.Errorf("WatchAttempts after 2nd: got %d, want 2", s.WatchAttempts)
	}
}

func TestIncrementWatchAttemptsFromNonZero(t *testing.T) {
	sf, _ := newTestStateFile(t)
	initial := writeInitialState(t, sf)
	initial.WatchAttempts = 5
	if err := sf.Write(initial); err != nil {
		t.Fatal(err)
	}

	if err := IncrementWatchAttempts(sf); err != nil {
		t.Fatalf("IncrementWatchAttempts: %v", err)
	}
	s, _ := sf.Read()
	if s.WatchAttempts != 6 {
		t.Errorf("WatchAttempts: got %d, want 6", s.WatchAttempts)
	}
}

func TestIncrementWatchAttemptsError(t *testing.T) {
	sf := NewStateFile("/nonexistent/dir/state.json")
	err := IncrementWatchAttempts(sf)
	if err == nil {
		t.Error("expected error for unwritable path, got nil")
	}
}

func TestResetToRetry(t *testing.T) {
	sf, _ := newTestStateFile(t)
	initial := writeInitialState(t, sf)
	initial.PhaseStatus = "blocked"
	initial.BlockedReason = "gate failed"
	initial.BlockedAt = "2026-03-09T12:00:00Z"
	if err := sf.Write(initial); err != nil {
		t.Fatal(err)
	}

	if err := ResetToRetry(sf); err != nil {
		t.Fatalf("ResetToRetry: %v", err)
	}

	s, _ := sf.Read()
	if s.PhaseStatus != "pending" {
		t.Errorf("PhaseStatus: got %q, want pending", s.PhaseStatus)
	}
	if s.BlockedReason != "" {
		t.Errorf("BlockedReason: got %q, want empty", s.BlockedReason)
	}
	if s.BlockedAt != "" {
		t.Errorf("BlockedAt: got %q, want empty", s.BlockedAt)
	}
}

func TestResetToRetryPreservesOtherFields(t *testing.T) {
	sf, _ := newTestStateFile(t)
	initial := writeInitialState(t, sf)
	initial.WatchAttempts = 2
	initial.Notes = "important"
	initial.PhaseStatus = "blocked"
	if err := sf.Write(initial); err != nil {
		t.Fatal(err)
	}

	if err := ResetToRetry(sf); err != nil {
		t.Fatalf("ResetToRetry: %v", err)
	}

	s, _ := sf.Read()
	if s.WatchAttempts != 2 {
		t.Errorf("WatchAttempts: got %d, want 2", s.WatchAttempts)
	}
	if s.Notes != "important" {
		t.Errorf("Notes: got %q, want 'important'", s.Notes)
	}
	if s.PhaseStatus != "pending" {
		t.Errorf("PhaseStatus: got %q, want pending", s.PhaseStatus)
	}
}

func TestResetToRetryIdempotent(t *testing.T) {
	sf, _ := newTestStateFile(t)
	initial := writeInitialState(t, sf)
	initial.PhaseStatus = "pending"
	initial.BlockedReason = ""
	initial.BlockedAt = ""
	if err := sf.Write(initial); err != nil {
		t.Fatal(err)
	}

	if err := ResetToRetry(sf); err != nil {
		t.Fatalf("ResetToRetry: %v", err)
	}

	s, _ := sf.Read()
	if s.PhaseStatus != "pending" {
		t.Errorf("PhaseStatus: got %q, want pending", s.PhaseStatus)
	}
}

func TestResetToRetryError(t *testing.T) {
	sf := NewStateFile("/nonexistent/dir/state.json")
	err := ResetToRetry(sf)
	if err == nil {
		t.Error("expected error for unwritable path, got nil")
	}
}

