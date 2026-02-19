package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

func setupEscalationState(t *testing.T, stuckIterations, rollbackCount int) (*state.StateFile, string) {
	t.Helper()

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phaseDir := filepath.Join(planDir, "phases", "test-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	ps := arc.NewPhaseState("test-plan", "test-phase", "feature")
	ps.StuckIterations = stuckIterations
	ps.RollbackCount = rollbackCount
	ps.PhaseStatus = "implementing"

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(phaseDir, "state.json")
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	sf := state.NewStateFile(statePath)
	return sf, plansDir
}

func TestHandleEscalationLowStuck(t *testing.T) {
	// stuck < 3: should be a no-op
	sf, plansDir := setupEscalationState(t, 1, 0)

	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}

	opts := RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	}

	err = handleEscalation(context.Background(), opts, sf, ps)
	if err != nil {
		t.Fatalf("expected no error for stuck=1, got: %v", err)
	}

	// State should be unchanged
	after, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if after.PhaseStatus != "implementing" {
		t.Fatalf("expected status 'implementing', got %q", after.PhaseStatus)
	}
}

func TestHandleEscalationMidStuck(t *testing.T) {
	// stuck >= 3 and < 6: should return nil (escalation instructions generated in main loop)
	sf, plansDir := setupEscalationState(t, 4, 0)

	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}

	opts := RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	}

	err = handleEscalation(context.Background(), opts, sf, ps)
	if err != nil {
		t.Fatalf("expected no error for stuck=4, got: %v", err)
	}
}

func TestHandleEscalationRollback(t *testing.T) {
	// stuck >= 6, rollbackCount < 2: should reset iteration and increment rollback
	sf, plansDir := setupEscalationState(t, 6, 0)

	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}

	opts := RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	}

	err = handleEscalation(context.Background(), opts, sf, ps)
	if err != nil {
		t.Fatalf("expected no error for rollback, got: %v", err)
	}

	after, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if after.RollbackCount != 1 {
		t.Fatalf("expected rollback count 1, got %d", after.RollbackCount)
	}
	if after.Iteration.Current != 0 {
		t.Fatalf("expected iteration reset to 0, got %d", after.Iteration.Current)
	}
	if after.StuckIterations != 0 {
		t.Fatalf("expected stuck iterations reset to 0, got %d", after.StuckIterations)
	}
	if after.TestsPassing != 0 {
		t.Fatalf("expected tests passing reset to 0, got %d", after.TestsPassing)
	}
	if after.TestsTotal != 0 {
		t.Fatalf("expected tests total reset to 0, got %d", after.TestsTotal)
	}
}

func TestHandleEscalationMaxRollbacksBlocked(t *testing.T) {
	// stuck >= 6, rollbackCount >= 2: should set status to "blocked"
	sf, plansDir := setupEscalationState(t, 6, 2)

	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}

	opts := RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	}

	err = handleEscalation(context.Background(), opts, sf, ps)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	after, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if after.PhaseStatus != "blocked" {
		t.Fatalf("expected status 'blocked', got %q", after.PhaseStatus)
	}
	if !after.Blocked.IsBlocked {
		t.Fatal("expected Blocked.IsBlocked to be true")
	}
	if after.Blocked.Reason == nil || *after.Blocked.Reason != "max rollbacks exhausted" {
		t.Fatalf("expected blocked reason 'max rollbacks exhausted', got %v", after.Blocked.Reason)
	}
}

func TestHandleEscalationSecondRollback(t *testing.T) {
	// stuck >= 6, rollbackCount = 1: should do second rollback
	sf, plansDir := setupEscalationState(t, 7, 1)

	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}

	opts := RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	}

	err = handleEscalation(context.Background(), opts, sf, ps)
	if err != nil {
		t.Fatalf("expected no error for second rollback, got: %v", err)
	}

	after, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if after.RollbackCount != 2 {
		t.Fatalf("expected rollback count 2, got %d", after.RollbackCount)
	}
	if after.Iteration.Current != 0 {
		t.Fatalf("expected iteration reset to 0, got %d", after.Iteration.Current)
	}
}
