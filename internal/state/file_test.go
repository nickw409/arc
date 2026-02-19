package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

func newTestStateFile(t *testing.T) (*StateFile, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	return NewStateFile(path), dir
}

func writeInitialState(t *testing.T, sf *StateFile) *arc.PhaseState {
	t.Helper()
	s := arc.NewPhaseState("p", "ph", "feature")
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	return s
}

func TestStateFileReadWriteRoundtrip(t *testing.T) {
	sf, _ := newTestStateFile(t)
	original := arc.NewPhaseState("p", "ph", "feature")

	if err := sf.Write(original); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	if got.Phase != original.Phase {
		t.Fatalf("Phase = %q, want %q", got.Phase, original.Phase)
	}
	if got.Plan != original.Plan {
		t.Fatalf("Plan = %q, want %q", got.Plan, original.Plan)
	}
	if got.WorkflowType != original.WorkflowType {
		t.Fatalf("WorkflowType = %q, want %q", got.WorkflowType, original.WorkflowType)
	}
	if got.PhaseStatus != original.PhaseStatus {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, original.PhaseStatus)
	}
}

func TestStateFileAtomicWrite(t *testing.T) {
	sf, dir := newTestStateFile(t)
	s := arc.NewPhaseState("p", "ph", "feature")
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Check no temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestStateFileUpdateMutation(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "implementing"
		return nil
	})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus != "implementing" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "implementing")
	}
}

func TestStateFileUpdateErrorNoWrite(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "bad"
		return errors.New("abort")
	})
	if err == nil {
		t.Fatal("expected error from Update, got nil")
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus == "bad" {
		t.Fatal("PhaseStatus should not have been updated to 'bad' after fn error")
	}
	if got.PhaseStatus != "pending" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "pending")
	}
}

func TestStateFileReadMissing(t *testing.T) {
	sf := NewStateFile("/nonexistent/state.json")
	_, err := sf.Read()
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestStateFileReadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	sf := NewStateFile(path)
	_, err := sf.Read()
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestStateFileReadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	sf := NewStateFile(path)
	_, err := sf.Read()
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}

func TestStateFilePath(t *testing.T) {
	sf := NewStateFile("/tmp/test/state.json")
	if sf.Path() != "/tmp/test/state.json" {
		t.Fatalf("Path() = %q, want %q", sf.Path(), "/tmp/test/state.json")
	}
}

func TestStateFileUpdateConcurrent(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := sf.Update(func(s *arc.PhaseState) error {
				s.GlobalIterations++
				return nil
			})
			if err != nil {
				t.Errorf("Update error: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.GlobalIterations != 10 {
		t.Fatalf("GlobalIterations = %d, want 10", got.GlobalIterations)
	}
}

func TestFlockUpdateNonexistentState(t *testing.T) {
	err := FlockUpdate("/tmp/nonexistent/state.json", func(s *arc.PhaseState) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for nonexistent state file, got nil")
	}
}

func TestFlockConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write initial state
	sf := NewStateFile(path)
	writeInitialState(t, sf)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := FlockUpdate(path, func(s *arc.PhaseState) error {
				s.GlobalIterations++
				return nil
			})
			if err != nil {
				t.Errorf("FlockUpdate error: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.GlobalIterations != 10 {
		t.Fatalf("GlobalIterations = %d, want 10", got.GlobalIterations)
	}
}

func TestFlockErrorReleasesLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	sf := NewStateFile(path)
	writeInitialState(t, sf)

	// First call: error in fn
	err := FlockUpdate(path, func(s *arc.PhaseState) error {
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error from FlockUpdate, got nil")
	}

	// Second call should not deadlock — lock was released despite error
	done := make(chan bool, 1)
	go func() {
		err := FlockUpdate(path, func(s *arc.PhaseState) error {
			s.PhaseStatus = "ok"
			return nil
		})
		if err != nil {
			t.Errorf("second FlockUpdate error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
		// success — lock was released
	case <-time.After(5 * time.Second):
		t.Fatal("second FlockUpdate deadlocked — lock was not released after error")
	}
}
