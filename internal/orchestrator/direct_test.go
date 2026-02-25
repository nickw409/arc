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

// setupDirectPlan creates a plan directory for direct plan tests.
func setupDirectPlan(t *testing.T, planName string, phases []string) (plansDir string, planDir string) {
	t.Helper()
	plansDir = t.TempDir()
	planDir = filepath.Join(plansDir, planName)

	for _, phase := range phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}

		ps := arc.NewPhaseState(planName, phase, "direct")
		data, err := json.MarshalIndent(ps, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# "+phase+"\nDo the task."), 0644); err != nil {
			t.Fatal(err)
		}
	}

	meta := arc.NewPlanMeta(planName, "direct", phases)
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	return plansDir, planDir
}

// markPhaseComplete sets phase_status=complete in state.json.
func markPhaseComplete(t *testing.T, planDir, phase string) {
	t.Helper()
	stateFile := filepath.Join(planDir, "phases", phase, "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var ps arc.PhaseState
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatal(err)
	}
	ps.PhaseStatus = "complete"
	out, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, out, 0644); err != nil {
		t.Fatal(err)
	}
}

func directTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestRunDirectPlanLoopAllComplete verifies that when the agent marks all phases
// complete (simulated by pre-marking state.json), runDirectPlanLoop returns nil.
func TestRunDirectPlanLoopAllComplete(t *testing.T) {
	_, planDir := setupDirectPlan(t, "dp-complete", []string{"alpha", "beta"})

	// Simulate agent having called `arc manage dp-complete <phase> complete`.
	markPhaseComplete(t, planDir, "alpha")
	markPhaseComplete(t, planDir, "beta")

	// Mock agent just needs to run and exit 0.
	t.Setenv("MOCK_OUTPUT", "All phases done.")

	meta := &arc.PlanMeta{
		Name:         "dp-complete",
		WorkflowType: "direct",
		Phases:       []string{"alpha", "beta"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runDirectPlanLoop(ctx, LaunchOptions{
		PlanName: "dp-complete",
		PlansDir: filepath.Dir(planDir),
		Logger:   directTestLogger(),
	}, planDir, meta, "")

	if err != nil {
		t.Fatalf("expected nil error when all phases complete, got: %v", err)
	}
}

// TestRunDirectPlanLoopNoneComplete verifies that when no phases are marked complete,
// runDirectPlanLoop marks them all blocked and returns an error.
func TestRunDirectPlanLoopNoneComplete(t *testing.T) {
	_, planDir := setupDirectPlan(t, "dp-none", []string{"alpha", "beta"})

	// Mock agent exits 0 but doesn't mark anything complete.
	t.Setenv("MOCK_OUTPUT", "Session output without completions.")

	meta := &arc.PlanMeta{
		Name:         "dp-none",
		WorkflowType: "direct",
		Phases:       []string{"alpha", "beta"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runDirectPlanLoop(ctx, LaunchOptions{
		PlanName: "dp-none",
		PlansDir: filepath.Dir(planDir),
		Logger:   directTestLogger(),
	}, planDir, meta, "")

	if err == nil {
		t.Fatal("expected error when phases not marked complete, got nil")
	}

	// Both phases should be blocked
	for _, phase := range []string{"alpha", "beta"} {
		data, readErr := os.ReadFile(filepath.Join(planDir, "phases", phase, "state.json"))
		if readErr != nil {
			t.Fatalf("reading state.json for %s: %v", phase, readErr)
		}
		var ps arc.PhaseState
		if err := json.Unmarshal(data, &ps); err != nil {
			t.Fatalf("unmarshaling state for %s: %v", phase, err)
		}
		if ps.PhaseStatus != "blocked" {
			t.Fatalf("phase %s: expected phase_status=blocked, got %q", phase, ps.PhaseStatus)
		}
		if !ps.Blocked.IsBlocked {
			t.Fatalf("phase %s: expected Blocked.IsBlocked=true", phase)
		}
	}
}

// TestRunDirectPlanLoopPartialComplete verifies that a phase already marked complete
// is preserved while incomplete phases are blocked.
func TestRunDirectPlanLoopPartialComplete(t *testing.T) {
	_, planDir := setupDirectPlan(t, "dp-partial", []string{"alpha", "beta"})

	// Alpha was completed by the agent; beta was not.
	markPhaseComplete(t, planDir, "alpha")

	t.Setenv("MOCK_OUTPUT", "Agent ran but only completed alpha.")

	meta := &arc.PlanMeta{
		Name:         "dp-partial",
		WorkflowType: "direct",
		Phases:       []string{"alpha", "beta"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runDirectPlanLoop(ctx, LaunchOptions{
		PlanName: "dp-partial",
		PlansDir: filepath.Dir(planDir),
		Logger:   directTestLogger(),
	}, planDir, meta, "")

	if err == nil {
		t.Fatal("expected error for partially completed plan, got nil")
	}

	// Alpha should still be complete
	alphaData, _ := os.ReadFile(filepath.Join(planDir, "phases", "alpha", "state.json"))
	var alphaState arc.PhaseState
	json.Unmarshal(alphaData, &alphaState)
	if alphaState.PhaseStatus != "complete" {
		t.Fatalf("alpha: expected phase_status=complete, got %q", alphaState.PhaseStatus)
	}

	// Beta should be blocked
	betaData, _ := os.ReadFile(filepath.Join(planDir, "phases", "beta", "state.json"))
	var betaState arc.PhaseState
	json.Unmarshal(betaData, &betaState)
	if betaState.PhaseStatus != "blocked" {
		t.Fatalf("beta: expected phase_status=blocked, got %q", betaState.PhaseStatus)
	}
}

// TestRunDirectPlanLoopCrashRetries verifies that on agent crash, runDirectPlanLoop
// retries once and then blocks all phases.
func TestRunDirectPlanLoopCrashRetries(t *testing.T) {
	scriptDir := t.TempDir()
	t.Setenv("MOCK_SCRIPT_DIR", scriptDir)
	// Write crash scripts — both calls will fail
	for i := 0; i < 4; i++ {
		path := filepath.Join(scriptDir, "call_"+string(rune('0'+i))+".txt")
		os.WriteFile(path, []byte("crash output"), 0644)
	}
	t.Setenv("MOCK_EXIT_CODE", "1")

	_, planDir := setupDirectPlan(t, "dp-crash", []string{"core"})

	meta := &arc.PlanMeta{
		Name:         "dp-crash",
		WorkflowType: "direct",
		Phases:       []string{"core"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runDirectPlanLoop(ctx, LaunchOptions{
		PlanName: "dp-crash",
		PlansDir: filepath.Dir(planDir),
		Logger:   directTestLogger(),
	}, planDir, meta, "")

	if err == nil {
		t.Fatal("expected error after crash, got nil")
	}

	// Core phase should be blocked
	data, _ := os.ReadFile(filepath.Join(planDir, "phases", "core", "state.json"))
	var ps arc.PhaseState
	json.Unmarshal(data, &ps)
	if ps.PhaseStatus != "blocked" {
		t.Fatalf("expected phase_status=blocked after crash, got %q", ps.PhaseStatus)
	}
}

// TestRunDirectPlanLoopContextCancel verifies that context cancellation is handled cleanly.
func TestRunDirectPlanLoopContextCancel(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "5000") // agent sleeps long
	t.Setenv("MOCK_OUTPUT", "never reached")

	_, planDir := setupDirectPlan(t, "dp-cancel", []string{"core"})

	meta := &arc.PlanMeta{
		Name:         "dp-cancel",
		WorkflowType: "direct",
		Phases:       []string{"core"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runDirectPlanLoop(ctx, LaunchOptions{
		PlanName: "dp-cancel",
		PlansDir: filepath.Dir(planDir),
		Logger:   directTestLogger(),
	}, planDir, meta, "")

	// Should return context error (not nil)
	if err == nil {
		t.Fatal("expected error from context cancel, got nil")
	}
}
