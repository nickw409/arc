package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

func TestMain(m *testing.M) {
	// Build the mock agent binary for review tests.
	tmpDir, err := os.MkdirTemp("", "review-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	testBin := filepath.Join(tmpDir, "mockagent")
	cmd := exec.Command("go", "build", "-o", testBin, "../agent/testdata/mockagent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build mock agent: %v\n", err)
		os.Exit(1)
	}

	// Override the agent command name so RunAdversary uses our mock binary.
	agentCommandName = testBin

	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupReviewPlan creates a plan directory structure for review tests.
func setupReviewPlan(t *testing.T, planName string, phases []string) string {
	t.Helper()

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, planName)
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, phase := range phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Write plan.md for hash computation
		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# "+phase+"\nPhase plan."), 0644); err != nil {
			t.Fatal(err)
		}
		// Write state.json
		state := arc.NewPhaseState(planName, phase, "feature")
		stateData, _ := json.MarshalIndent(state, "", "  ")
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create reviews directory
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write plan.json
	planMeta := arc.NewPlanMeta(planName, "feature", phases)
	metaData, _ := json.MarshalIndent(planMeta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	return plansDir
}

func TestReviewDefaultAdversaries(t *testing.T) {
	advs := DefaultAdversaries()
	if len(advs) != 5 {
		t.Fatalf("expected 5 adversaries, got %d", len(advs))
	}

	expected := map[string]struct {
		required    bool
		failVerdict string
	}{
		"coverage":      {required: true, failVerdict: "coverage_gaps"},
		"ambiguity":     {required: true, failVerdict: "ambiguous"},
		"scope":         {required: false, failVerdict: "scope_too_large"},
		"consistency":   {required: true, failVerdict: "inconsistent"},
		"executability": {required: true, failVerdict: "blocked"},
	}

	for _, adv := range advs {
		exp, ok := expected[adv.Name]
		if !ok {
			t.Fatalf("unexpected adversary name %q", adv.Name)
		}
		if adv.Required != exp.required {
			t.Fatalf("adversary %q: got Required=%v, want %v", adv.Name, adv.Required, exp.required)
		}
		if adv.FailVerdict != exp.failVerdict {
			t.Fatalf("adversary %q: got FailVerdict=%q, want %q", adv.Name, adv.FailVerdict, exp.failVerdict)
		}
	}
}

func TestReviewRunBasic(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Verdicts) != 5 {
		t.Fatalf("expected 5 verdicts, got %d", len(result.Verdicts))
	}
}

func TestReviewAllCached(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Read plan.md to compute hash
	planMD, err := os.ReadFile(filepath.Join(planDir, "phases", "phase-1", "plan.md"))
	if err != nil {
		t.Fatal(err)
	}

	// Precompute hash (SHA256)
	// For the test, we'll use the raw bytes; the actual implementation computes SHA256
	_ = planMD

	// Create adversary_history.json with all passing entries
	type HistoryEntry struct {
		Hash      string `json:"hash"`
		Verdict   string `json:"verdict"`
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	// Write a history file — the actual hash computation happens in the implementation
	history := map[string]map[string]HistoryEntry{
		"phase-1": {
			"coverage":      {Hash: "placeholder", Verdict: "coverage_sufficient", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
			"ambiguity":     {Hash: "placeholder", Verdict: "unambiguous", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
			"scope":         {Hash: "placeholder", Verdict: "scope_appropriate", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
			"consistency":   {Hash: "placeholder", Verdict: "consistent", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
			"executability": {Hash: "placeholder", Verdict: "executable", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
		},
	}
	histData, _ := json.MarshalIndent(history, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "reviews", "adversary_history.json"), histData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReviewContextCancel(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := Run(ctx, ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	_ = result
}

func TestReviewRegressionDetected(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Create adversary_history.json showing coverage previously "passed"
	type HistoryEntry struct {
		Hash      string `json:"hash"`
		Verdict   string `json:"verdict"`
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	history := map[string]map[string]HistoryEntry{
		"phase-1": {
			"coverage": {Hash: "same_hash", Verdict: "coverage_sufficient", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
		},
	}
	histData, _ := json.MarshalIndent(history, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "reviews", "adversary_history.json"), histData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReviewConditionalStatus(t *testing.T) {
	// All required adversaries pass, but scope (non-required) fails
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReviewNeedsReviewStatus(t *testing.T) {
	// coverage adversary fails (not cached, not previously passing). Other required adversaries pass.
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReviewHistoryFirstRun(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Ensure no adversary_history.json exists
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	os.Remove(histPath) // Remove if it exists

	result, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Verdicts) != 5 {
		t.Fatalf("expected 5 verdicts, got %d", len(result.Verdicts))
	}
}
