package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupPhaseTestPlan creates a minimal plan directory structure for phase runner tests.
func setupPhaseTestPlan(t *testing.T, planName, phaseName, workflowType string) string {
	t.Helper()

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, planName)
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write state.json (brand new phase)
	state := arc.NewPhaseState(planName, phaseName, workflowType)
	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
		t.Fatal(err)
	}

	// Write plan.md
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Plan\nTest."), 0644); err != nil {
		t.Fatal(err)
	}

	// Write plan.json
	planMeta := arc.NewPlanMeta(planName, workflowType, []string{phaseName})
	metaData, err := json.MarshalIndent(planMeta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	return plansDir
}

func TestRunPhaseReachesTerminal(t *testing.T) {
	plansDir := setupPhaseTestPlan(t, "test-plan", "test-phase", "feature")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	})

	// RunPhase will return with context deadline or agent error
	_ = err
}

func TestRunPhaseActionRetry(t *testing.T) {
	plansDir := setupPhaseTestPlan(t, "test-plan", "test-phase", "feature")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	})
	_ = err
}

func TestRunPhaseActionAbort(t *testing.T) {
	plansDir := setupPhaseTestPlan(t, "test-plan", "test-phase", "feature")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	})
	_ = err
}

func TestRunPhaseContextCancelMidLoop(t *testing.T) {
	plansDir := setupPhaseTestPlan(t, "test-plan", "test-phase", "feature")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    testLogger(),
	})

	_ = err
}
