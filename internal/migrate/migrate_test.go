package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestMigrateStateMissingFields(t *testing.T) {
	input := `{"phase": "qa", "plan": "old-plan", "phase_status": "implementing"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Phase != "qa" {
		t.Errorf("Phase=%q, want 'qa'", ps.Phase)
	}
	if ps.Iteration.Max != 25 {
		t.Errorf("Iteration.Max=%d, want 25", ps.Iteration.Max)
	}
	if ps.Packages == nil {
		t.Error("Packages is nil, want empty slice")
	}
	if ps.Disputes == nil {
		t.Error("Disputes is nil, want empty slice")
	}
}

func TestMigrateStateStringNull(t *testing.T) {
	input := `{"phase": "test", "plan": "p", "parallel_execution": "null"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.ParallelExecution != nil {
		t.Error("ParallelExecution should be nil for string 'null'")
	}
}

func TestMigrateStateEmptyJSON(t *testing.T) {
	ps, err := MigrateState([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if ps.WorkflowType != "feature" {
		t.Errorf("WorkflowType=%q, want 'feature'", ps.WorkflowType)
	}
	if ps.Iteration.Max != 25 {
		t.Errorf("Iteration.Max=%d, want 25", ps.Iteration.Max)
	}
}

func TestMigrateStateWrongType(t *testing.T) {
	input := `{"tests_passing": "five"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.TestsPassing != 0 {
		t.Errorf("TestsPassing=%d, want 0", ps.TestsPassing)
	}
}

func TestMigrateStateExtraFields(t *testing.T) {
	input := `{"phase": "test", "future_field": "value"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Phase != "test" {
		t.Errorf("Phase=%q, want 'test'", ps.Phase)
	}
}

func TestMigrateStateMissingWorkflowType(t *testing.T) {
	input := `{"phase": "test"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.WorkflowType != "feature" {
		t.Errorf("WorkflowType=%q, want 'feature'", ps.WorkflowType)
	}
}

func TestMigrateStateNullArrays(t *testing.T) {
	input := `{"disputes": null, "test_files": null}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Disputes == nil {
		t.Error("Disputes is nil, want empty slice")
	}
	if ps.TestFiles == nil {
		t.Error("TestFiles is nil, want empty slice")
	}
}

func TestMigrateStateNestedWrongType(t *testing.T) {
	input := `{"iteration": "not an object"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Iteration.Max != 25 {
		t.Errorf("Iteration.Max=%d, want 25 (default)", ps.Iteration.Max)
	}
}

func TestMigrateStateFloat64Integers(t *testing.T) {
	input := `{"tests_passing": 5.0, "tests_total": 10.0}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.TestsPassing != 5 {
		t.Errorf("TestsPassing=%d, want 5", ps.TestsPassing)
	}
	if ps.TestsTotal != 10 {
		t.Errorf("TestsTotal=%d, want 10", ps.TestsTotal)
	}
}

func TestMigrateStateInterventionNullString(t *testing.T) {
	input := `{"intervention_request": "null"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.InterventionRequest != nil {
		t.Error("InterventionRequest should be nil for string 'null'")
	}
}

func TestMigrateStateStringNullOnSlice(t *testing.T) {
	input := `{"verdicts_history": "null"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.VerdictsHistory == nil {
		t.Error("VerdictsHistory is nil, want empty slice")
	}
	if len(ps.VerdictsHistory) != 0 {
		t.Errorf("VerdictsHistory len=%d, want 0", len(ps.VerdictsHistory))
	}
}

func TestMigrateStateMissingPhaseField(t *testing.T) {
	input := `{"plan": "my-plan", "workflow_type": "feature"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Phase != "" {
		t.Errorf("Phase=%q, want empty string", ps.Phase)
	}
}

func TestMigrateStateBlockedNullString(t *testing.T) {
	input := `{"blocked": "null"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Should be zero-value BlockedInfo
	if ps.Blocked.IsBlocked {
		t.Error("Blocked.IsBlocked should be false")
	}
}

func TestMigrateStateJSONArray(t *testing.T) {
	_, err := MigrateState([]byte(`[]`))
	if err == nil {
		t.Fatal("expected error for JSON array input")
	}
}

func TestMigrateStateDeeplyNestedWrongType(t *testing.T) {
	input := `{"chunks": "not an object"}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Should default to zero-value Chunks with initialized slices
	if ps.Chunks.Completed == nil {
		t.Error("Chunks.Completed is nil")
	}
}

func TestMigrateStateNestedWrongTypeIteration(t *testing.T) {
	input := `{"iteration": {"current": "not_a_number", "max": 25}}`
	ps, err := MigrateState([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if ps.Iteration.Current != 0 {
		t.Errorf("Iteration.Current=%d, want 0", ps.Iteration.Current)
	}
}

// --- Plan-level migration tests ---

func setupTestPlan(t *testing.T, planName string, phases []string, states map[string]string) string {
	t.Helper()
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, planName)
	phasesDir := filepath.Join(planDir, "phases")

	for _, phase := range phases {
		phaseDir := filepath.Join(phasesDir, phase)
		os.MkdirAll(phaseDir, 0755)

		stateJSON := states[phase]
		if stateJSON == "" {
			ps := arc.NewPhaseState(planName, phase, "feature")
			data, _ := json.MarshalIndent(ps, "", "  ")
			stateJSON = string(data)
		}
		os.WriteFile(filepath.Join(phaseDir, "state.json"), []byte(stateJSON), 0644)
	}

	// Write plan.json
	meta := arc.NewPlanMeta(planName, "feature", phases)
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)

	return plansDir
}

func TestMigratePlanBackupCreated(t *testing.T) {
	plansDir := setupTestPlan(t, "test-plan", []string{"core"}, map[string]string{
		"core": `{"phase": "core", "plan": "test-plan", "phase_status": "implementing"}`,
	})
	planDir := filepath.Join(plansDir, "test-plan")

	if err := MigratePlan(planDir, false); err != nil {
		t.Fatal(err)
	}

	bakPath := filepath.Join(planDir, "phases", "core", "state.json.bak")
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		t.Fatal("backup file not created")
	}
}

func TestMigrateDryRunNoWrites(t *testing.T) {
	plansDir := setupTestPlan(t, "test-plan", []string{"core"}, map[string]string{
		"core": `{"phase": "core", "plan": "test-plan", "phase_status": "implementing"}`,
	})
	planDir := filepath.Join(plansDir, "test-plan")

	if err := MigratePlan(planDir, true); err != nil {
		t.Fatal(err)
	}

	bakPath := filepath.Join(planDir, "phases", "core", "state.json.bak")
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Fatal("backup should not exist in dry-run mode")
	}
}

func TestMigrateAlreadyCurrent(t *testing.T) {
	// Use a properly formatted state
	ps := arc.NewPhaseState("test-plan", "core", "feature")
	data, _ := json.MarshalIndent(ps, "", "  ")

	plansDir := setupTestPlan(t, "test-plan", []string{"core"}, map[string]string{
		"core": string(data),
	})
	planDir := filepath.Join(plansDir, "test-plan")

	if err := MigratePlan(planDir, false); err != nil {
		t.Fatal(err)
	}

	// No backup should be created since nothing changed
	bakPath := filepath.Join(planDir, "phases", "core", "state.json.bak")
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Fatal("backup should not exist for already-current state")
	}
}

func TestMigrateCorruptedState(t *testing.T) {
	plansDir := setupTestPlan(t, "test-plan", []string{"core"}, map[string]string{
		"core": `{invalid`,
	})

	result, err := Migrate(MigrateOptions{PlansDir: plansDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected errors for corrupted state")
	}
}

func TestMigratePartialPlan(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phasesDir := filepath.Join(planDir, "phases")

	// Phase 1: valid old format
	os.MkdirAll(filepath.Join(phasesDir, "phase1"), 0755)
	os.WriteFile(filepath.Join(phasesDir, "phase1", "state.json"),
		[]byte(`{"phase": "phase1", "plan": "test-plan"}`), 0644)

	// Phase 2: corrupted
	os.MkdirAll(filepath.Join(phasesDir, "phase2"), 0755)
	os.WriteFile(filepath.Join(phasesDir, "phase2", "state.json"),
		[]byte(`{invalid`), 0644)

	// Phase 3: valid
	os.MkdirAll(filepath.Join(phasesDir, "phase3"), 0755)
	os.WriteFile(filepath.Join(phasesDir, "phase3", "state.json"),
		[]byte(`{"phase": "phase3", "plan": "test-plan"}`), 0644)

	err := MigratePlan(planDir, false)
	// Should error because of phase2
	if err == nil {
		t.Fatal("expected error for corrupted phase")
	}
}

func TestMigrateEmptyPlansDir(t *testing.T) {
	emptyDir := t.TempDir()
	result, err := Migrate(MigrateOptions{PlansDir: emptyDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.PlansFound != 0 {
		t.Errorf("PlansFound=%d, want 0", result.PlansFound)
	}
}

func TestMigratePlansDirNonexistent(t *testing.T) {
	_, err := Migrate(MigrateOptions{PlansDir: "/nonexistent/path"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestMigratePlanNoPhaseDir(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	os.MkdirAll(planDir, 0755)
	// No phases/ directory

	err := MigratePlan(planDir, false)
	if err == nil {
		t.Fatal("expected error for missing phases directory")
	}
}

func TestMigratePlanMissingStateJSON(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phaseDir := filepath.Join(planDir, "phases", "core")
	os.MkdirAll(phaseDir, 0755)
	// No state.json file

	err := MigratePlan(planDir, false)
	// Should succeed — phase without state.json is skipped
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigratePlanDirect(t *testing.T) {
	plansDir := setupTestPlan(t, "test-plan", []string{"core", "api"}, map[string]string{
		"core": `{"phase": "core", "plan": "test-plan"}`,
		"api":  `{"phase": "api", "plan": "test-plan"}`,
	})
	planDir := filepath.Join(plansDir, "test-plan")

	if err := MigratePlan(planDir, false); err != nil {
		t.Fatal(err)
	}

	// Both should have backups
	for _, phase := range []string{"core", "api"} {
		bakPath := filepath.Join(planDir, "phases", phase, "state.json.bak")
		if _, err := os.Stat(bakPath); os.IsNotExist(err) {
			t.Errorf("backup not created for phase %s", phase)
		}
	}
}

func TestMigratePlansNonDirectoryEntries(t *testing.T) {
	plansDir := t.TempDir()
	// Create a stray file
	os.WriteFile(filepath.Join(plansDir, "stray-file.txt"), []byte("hi"), 0644)
	// Create a valid plan
	planDir := filepath.Join(plansDir, "test-plan")
	phasesDir := filepath.Join(planDir, "phases", "core")
	os.MkdirAll(phasesDir, 0755)
	ps := arc.NewPhaseState("test-plan", "core", "feature")
	data, _ := json.MarshalIndent(ps, "", "  ")
	os.WriteFile(filepath.Join(phasesDir, "state.json"), data, 0644)

	result, err := Migrate(MigrateOptions{PlansDir: plansDir})
	if err != nil {
		t.Fatal(err)
	}
	// Only the directory should be counted
	if result.PlansFound != 1 {
		t.Errorf("PlansFound=%d, want 1", result.PlansFound)
	}
}
