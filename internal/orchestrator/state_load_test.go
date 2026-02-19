package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestLoadPhaseStateValid(t *testing.T) {
	planDir := t.TempDir()
	phaseDir := filepath.Join(planDir, "phases", "my-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	ps := arc.NewPhaseState("test-plan", "my-phase", "feature")
	ps.PhaseStatus = "implementing"
	ps.TestsPassing = 3
	ps.TestsTotal = 5

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result := loadPhaseState(planDir, "my-phase")
	if result == nil {
		t.Fatal("expected non-nil phase state")
	}
	if result.PhaseStatus != "implementing" {
		t.Fatalf("expected status 'implementing', got %q", result.PhaseStatus)
	}
	if result.TestsPassing != 3 {
		t.Fatalf("expected 3 tests passing, got %d", result.TestsPassing)
	}
}

func TestLoadPhaseStateMissing(t *testing.T) {
	planDir := t.TempDir()

	result := loadPhaseState(planDir, "nonexistent")
	if result != nil {
		t.Fatal("expected nil for missing phase state")
	}
}

func TestLoadPhaseStateInvalidJSON(t *testing.T) {
	planDir := t.TempDir()
	phaseDir := filepath.Join(planDir, "phases", "bad-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	result := loadPhaseState(planDir, "bad-phase")
	if result != nil {
		t.Fatal("expected nil for invalid JSON state")
	}
}

func TestLoadAllPhaseStates(t *testing.T) {
	planDir := t.TempDir()
	phases := []string{"phase-a", "phase-b", "phase-c"}

	// Create state for phase-a and phase-b only
	for _, phase := range []string{"phase-a", "phase-b"} {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}

		ps := arc.NewPhaseState("test-plan", phase, "feature")
		ps.PhaseStatus = "complete"
		data, err := json.MarshalIndent(ps, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := loadAllPhaseStates(planDir, phases)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries in result map, got %d", len(result))
	}

	if result["phase-a"] == nil {
		t.Fatal("expected non-nil state for phase-a")
	}
	if result["phase-b"] == nil {
		t.Fatal("expected non-nil state for phase-b")
	}
	if result["phase-c"] != nil {
		t.Fatal("expected nil state for phase-c (no state file)")
	}
}

func TestLoadAllPhaseStatesEmpty(t *testing.T) {
	planDir := t.TempDir()

	result := loadAllPhaseStates(planDir, []string{})
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}
