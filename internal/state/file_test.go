package state

import (
	"encoding/json"
	"errors"
	"fmt"
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
				s.WatchAttempts++
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
	if got.WatchAttempts != 10 {
		t.Fatalf("WatchAttempts = %d, want 10", got.WatchAttempts)
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
				s.WatchAttempts++
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
	if got.WatchAttempts != 10 {
		t.Fatalf("WatchAttempts = %d, want 10", got.WatchAttempts)
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

// --- Schema version tests ---

func TestWriteStampsSchemaVersion(t *testing.T) {
	sf, _ := newTestStateFile(t)
	s := arc.NewPhaseState("p", "ph", "feature")
	// Ensure schema version starts at zero before write.
	s.SchemaVersion = 0
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, CurrentSchemaVersion)
	}
}

func TestReadV0StateCompatible(t *testing.T) {
	// A state file without schema_version (v0) should be read without error.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// Write a minimal valid state with schema_version absent (zero value omitted).
	raw := `{"phase":"ph","plan":"p","workflow_type":"feature","phase_status":"pending"}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sf := NewStateFile(path)
	got, err := sf.Read()
	if err != nil {
		t.Fatalf("expected no error for v0 state, got: %v", err)
	}
	if got.SchemaVersion != 0 {
		t.Fatalf("SchemaVersion = %d, want 0 (v0 file)", got.SchemaVersion)
	}
	if got.Phase != "ph" {
		t.Fatalf("Phase = %q, want %q", got.Phase, "ph")
	}
}

func TestReadFutureSchemaVersionReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// Write a state file with a schema version higher than the current one.
	futureVersion := CurrentSchemaVersion + 1
	raw := fmt.Sprintf(`{"schema_version":%d,"phase":"ph","plan":"p","workflow_type":"feature","phase_status":"pending"}`, futureVersion)
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sf := NewStateFile(path)
	_, err := sf.Read()
	if err == nil {
		t.Fatal("expected error for future schema version, got nil")
	}
	if !strings.Contains(err.Error(), "upgrade arc") {
		t.Fatalf("error should mention 'upgrade arc', got: %v", err)
	}
}

func TestReadCurrentSchemaVersionOK(t *testing.T) {
	sf, _ := newTestStateFile(t)
	s := arc.NewPhaseState("p", "ph", "feature")
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Reading back a file written by Write (with CurrentSchemaVersion stamped)
	// should succeed without error.
	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, CurrentSchemaVersion)
	}
}

func TestUpdateStampsSchemaVersion(t *testing.T) {
	sf, _ := newTestStateFile(t)
	// Write initial state without schema version via direct file write.
	s := arc.NewPhaseState("p", "ph", "feature")
	s.SchemaVersion = 0
	if err := sf.Write(s); err != nil {
		t.Fatalf("initial Write: %v", err)
	}

	// Now update — should stamp the schema version.
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "running"
		return nil
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d after Update", got.SchemaVersion, CurrentSchemaVersion)
	}
}

// --- CRC32 checksum tests ---

func TestWriteEmbedsChecksum(t *testing.T) {
	sf, _ := newTestStateFile(t)
	s := arc.NewPhaseState("p", "ph", "feature")
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.Checksum == "" {
		t.Fatal("expected non-empty checksum after Write, got empty string")
	}
	// Checksum should be exactly 8 hex characters.
	if len(got.Checksum) != 8 {
		t.Fatalf("expected 8-character checksum, got %q (len %d)", got.Checksum, len(got.Checksum))
	}
}

func TestReadVerifiesChecksumOK(t *testing.T) {
	sf, _ := newTestStateFile(t)
	s := arc.NewPhaseState("p", "ph", "feature")
	s.Notes = "round-trip check"
	if err := sf.Write(s); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Read should succeed and return correct data.
	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.Notes != "round-trip check" {
		t.Fatalf("Notes = %q, want %q", got.Notes, "round-trip check")
	}
}

func TestReadFallsBackOnChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	sf := NewStateFile(path)

	// Write a valid state so a .bak file is produced on the second write.
	s := arc.NewPhaseState("p", "ph", "feature")
	s.Notes = "good state"
	if err := sf.Write(s); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	// Write again — first write becomes .bak, second becomes primary.
	s2 := arc.NewPhaseState("p", "ph", "feature")
	s2.Notes = "second write"
	if err := sf.Write(s2); err != nil {
		t.Fatalf("second Write error: %v", err)
	}

	// Verify .bak exists (it should contain the first write, "good state").
	bakPath := path + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Fatalf(".bak file not created: %v", err)
	}

	// Tamper with the primary file's checksum field.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	m["checksum"] = "deadbeef" // corrupt the checksum
	corrupted, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, corrupted, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read should fall back to .bak (which has "good state").
	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read should succeed via fallback, got error: %v", err)
	}
	if got.Notes != "good state" {
		t.Fatalf("expected fallback to .bak with Notes=%q, got %q", "good state", got.Notes)
	}
}

func TestReadOldFileWithoutChecksumOK(t *testing.T) {
	// A file without the checksum field (old format) should read fine.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	raw := `{"schema_version":1,"phase":"ph","plan":"p","workflow_type":"feature","phase_status":"pending","iteration":{"current":0,"max":25},"chunks":{"total":0,"completed":[],"current":null,"remaining":[]},"blocked":{"is_blocked":false,"reason":null},"packages":[],"tests_passing":0,"tests_total":0,"stuck_iterations":0,"hang_count":0,"disputes":[],"last_cleared_disputes":[],"last_reviewed_iteration":0,"last_qa_reviewed_iteration":0,"verdicts_history":[],"last_verdict":"","test_files":[],"executed_escalations":[],"rollback_count":0,"global_iterations":0,"usage":{}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sf := NewStateFile(path)
	got, err := sf.Read()
	if err != nil {
		t.Fatalf("expected no error for old file without checksum, got: %v", err)
	}
	if got.Phase != "ph" {
		t.Fatalf("Phase = %q, want %q", got.Phase, "ph")
	}
	if got.Checksum != "" {
		t.Fatalf("expected empty Checksum for old file, got %q", got.Checksum)
	}
}
