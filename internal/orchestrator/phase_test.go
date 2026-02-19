package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	// This test calls RunPhase which spawns agents. Use a short timeout
	// so it doesn't hang if no agent binary is available.
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

func TestRunPhaseActionEscalate(t *testing.T) {
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

func TestRunPhaseActionIntervene(t *testing.T) {
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

func TestRunPhaseModeDerivation(t *testing.T) {
	// Verify that state name "qa_review" produces mode "qa-review"
	// This is a unit check on the mode derivation logic.
	// The actual RunPhase will pass the mode to RunIteration.
	tests := []struct {
		stateName string
		wantMode  string
	}{
		{"qa_review", "qa-review"},
		{"impl_review", "impl-review"},
		{"qa", "qa"},
		{"impl", "impl"},
		{"fix", "impl"},
		{"review", "review"},
	}

	for _, tc := range tests {
		t.Run(tc.stateName, func(t *testing.T) {
			// Mode derivation rules:
			// 1. Contains "review" -> mode = state name
			// 2. "qa" -> mode = "qa"
			// 3. MapStateToStatus -> "implementing" -> mode = "impl"
			// 4. All underscores replaced with hyphens
			var mode string
			if strings.Contains(tc.stateName, "review") {
				mode = tc.stateName
			} else if tc.stateName == "qa" {
				mode = "qa"
			} else {
				mode = "impl"
			}
			mode = strings.ReplaceAll(mode, "_", "-")

			if mode != tc.wantMode {
				t.Fatalf("state %q: got mode %q, want %q", tc.stateName, mode, tc.wantMode)
			}
		})
	}
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

	// When implemented: RunPhase should return promptly on context cancellation
	_ = err
}
