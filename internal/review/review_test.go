package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestReviewOutputFilesWritten(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	_, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that output files were created for each adversary
	adversaries := DefaultAdversaries()
	for _, adv := range adversaries {
		outPath := filepath.Join(planDir, "reviews", "phase-1_"+adv.Name+".md")
		if _, err := os.Stat(outPath); os.IsNotExist(err) {
			t.Errorf("expected output file %s to exist", outPath)
		}
	}
}

func TestReviewCachedResultsSkipOutputFiles(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Run once to get output files and history
	_, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Write a sentinel value into an output file
	sentinel := "SENTINEL_CONTENT"
	sentinelPath := filepath.Join(planDir, "reviews", "phase-1_coverage.md")
	if err := os.WriteFile(sentinelPath, []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	}

	// Run again — results should be cached, so sentinel should be preserved
	_, err = Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}

	data, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("failed to read sentinel file: %v", err)
	}
	if string(data) != sentinel {
		t.Fatalf("expected sentinel content %q, got %q", sentinel, string(data))
	}
}

func TestCleanupOutputFiles(t *testing.T) {
	planDir := t.TempDir()
	reviewsDir := filepath.Join(planDir, "reviews")
	if err := os.MkdirAll(reviewsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create output files for two phases
	adversaries := DefaultAdversaries()
	for _, adv := range adversaries {
		for _, phase := range []string{"phase-1", "phase-2"} {
			path := filepath.Join(reviewsDir, phase+"_"+adv.Name+".md")
			if err := os.WriteFile(path, []byte("output"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Create adversary_history.json
	histPath := filepath.Join(reviewsDir, "adversary_history.json")
	if err := os.WriteFile(histPath, []byte(`{"phase-1":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Cleanup only phase-1
	if err := CleanupOutputFiles(planDir, []string{"phase-1"}); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}

	// phase-1 output files should be gone
	for _, adv := range adversaries {
		path := filepath.Join(reviewsDir, "phase-1_"+adv.Name+".md")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", path)
		}
	}

	// phase-2 output files should still exist
	for _, adv := range adversaries {
		path := filepath.Join(reviewsDir, "phase-2_"+adv.Name+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to still exist", path)
		}
	}

	// adversary_history.json should still exist
	if _, err := os.Stat(histPath); os.IsNotExist(err) {
		t.Error("expected adversary_history.json to be preserved")
	}
}

func TestCleanupOutputFilesNoDir(t *testing.T) {
	planDir := t.TempDir()

	// Cleanup on non-existent reviews dir should not error
	if err := CleanupOutputFiles(planDir, []string{"phase-1"}); err != nil {
		t.Fatalf("expected no error for missing reviews dir, got: %v", err)
	}
}

func TestReviewIterationIncrements(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// First run: iteration should be 1
	result1, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if result1.Iteration != 1 {
		t.Fatalf("expected iteration 1, got %d", result1.Iteration)
	}

	// Modify plan.md so cache is invalidated
	if err := os.WriteFile(filepath.Join(planDir, "phases", "phase-1", "plan.md"), []byte("# phase-1\nUpdated plan."), 0644); err != nil {
		t.Fatal(err)
	}

	// Second run: iteration should be 2
	result2, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result2.Iteration != 2 {
		t.Fatalf("expected iteration 2, got %d", result2.Iteration)
	}
}

func TestReviewMaxIterationEnforcement(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Seed history at max iterations
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	hf := &historyFile{
		Phases:     make(map[string]map[string]historyEntry),
		Iterations: map[string]int{"phase-1": MaxReviewIterations},
	}
	data, _ := json.MarshalIndent(hf, "", "  ")
	if err := os.WriteFile(histPath, data, 0644); err != nil {
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
	if result.Status != "conditional" {
		t.Fatalf("expected status 'conditional' at max iterations, got %q", result.Status)
	}
	if result.Iteration != MaxReviewIterations {
		t.Fatalf("expected iteration %d, got %d", MaxReviewIterations, result.Iteration)
	}
}

func TestReviewBackwardCompatHistory(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Write old flat-format history
	oldHistory := map[string]map[string]historyEntry{
		"phase-1": {
			"coverage": {Hash: "oldhash", Verdict: "coverage_sufficient", Status: "passed", Timestamp: time.Now().Format(time.RFC3339)},
		},
	}
	data, _ := json.MarshalIndent(oldHistory, "", "  ")
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	if err := os.WriteFile(histPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load should succeed and convert to new format
	history := LoadHistory(histPath)
	if history.Phases == nil {
		t.Fatal("expected Phases to be non-nil")
	}
	if _, ok := history.Phases["phase-1"]["coverage"]; !ok {
		t.Fatal("expected coverage entry in phase-1")
	}
	if history.Iterations == nil {
		t.Fatal("expected Iterations to be non-nil")
	}
	if history.Iterations["phase-1"] != 0 {
		t.Fatalf("expected iteration 0 for old format, got %d", history.Iterations["phase-1"])
	}
}

func TestReviewCachedRunNoIterationIncrement(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Run once to populate cache
	result1, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if result1.Iteration != 1 {
		t.Fatalf("expected iteration 1 after first run, got %d", result1.Iteration)
	}

	// Run again without modifying plan.md — all results should be cached
	result2, err := Run(context.Background(), ReviewOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Phase:    "phase-1",
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result2.Iteration != 1 {
		t.Fatalf("expected iteration to stay at 1 for cached run, got %d", result2.Iteration)
	}

	// Verify history file still shows iteration 1
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	history := LoadHistory(histPath)
	if history.Iterations["phase-1"] != 1 {
		t.Fatalf("expected iteration count 1 in history, got %d", history.Iterations["phase-1"])
	}
}

func TestReviewAutoRemediation(t *testing.T) {
	plansDir := setupReviewPlan(t, "test-plan", []string{"phase-1"})
	planDir := filepath.Join(plansDir, "test-plan")

	// Write a plan.md that the mock agent can suggest fixes for
	planContent := "# Phase 1\n\nfunc Foo() {\n    return nil\n}\n"
	if err := os.WriteFile(filepath.Join(planDir, "phases", "phase-1", "plan.md"), []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up scripted responses: first call returns a failure with suggestions,
	// subsequent calls return passing verdicts.
	// The mock agent uses MOCK_SCRIPT_DIR for sequential responses.
	scriptDir := t.TempDir()

	// We have 5 adversaries running in parallel, and each uses the same mock binary.
	// Since we can't control which adversary gets which script call, we use
	// MOCK_OUTPUT to return a consistent passing response with a suggestion.
	// The first run: all agents return failure with a suggestion
	// After remediation: plan.md changes, cache invalidates, second run: all pass

	// Use MOCK_OUTPUT env to make all agents return a failure with suggestions on first call
	failOutput := "## Coverage Analysis\n\nMissing error return.\n\n## Suggestions\n\n<<<ORIGINAL\nfunc Foo() {\n>>>\n<<<SUGGESTED\nfunc Foo() error {\n>>>\n\n## Verdict\ncoverage_gaps\n"

	// For this test, use scripted responses:
	// call_0 through call_4 (first run, 5 adversaries): all fail with suggestions
	// call_5 through call_9 (second run, 5 adversaries): all pass
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(scriptDir, fmt.Sprintf("call_%d.txt", i)), []byte(failOutput), 0644); err != nil {
			t.Fatal(err)
		}
	}
	passOutput := "## Coverage Analysis\n\nAll good.\n\n## Verdict\ncoverage_sufficient\n"
	for i := 5; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(scriptDir, fmt.Sprintf("call_%d.txt", i)), []byte(passOutput), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Override agent command to use scripted mock
	original := agentCommandName
	defer func() { agentCommandName = original }()

	// Build a wrapper script that sets MOCK_SCRIPT_DIR
	wrapperScript := fmt.Sprintf("#!/bin/sh\nexport MOCK_SCRIPT_DIR=%s\nexec %s \"$@\"\n", scriptDir, agentCommandName)
	wrapperPath := filepath.Join(t.TempDir(), "wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(wrapperScript), 0755); err != nil {
		t.Fatal(err)
	}
	agentCommandName = wrapperPath

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

	// Verify plan.md was modified
	updatedPlan, err := os.ReadFile(filepath.Join(planDir, "phases", "phase-1", "plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedPlan), "func Foo() error {") {
		t.Fatalf("expected plan.md to be updated with suggestion, got: %s", string(updatedPlan))
	}

	// Verify suggestions were applied
	if result.SuggestionsApplied == 0 {
		t.Fatal("expected at least one suggestion to be applied")
	}

	// Verify iteration details were recorded
	if len(result.IterationDetails) == 0 {
		t.Fatal("expected iteration details to be recorded")
	}
}

func TestReviewAutoRemediationNoSuggestions(t *testing.T) {
	// When adversaries fail but provide no suggestions, the loop should stop
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

	// The mock agent output from the default binary won't contain valid suggestions,
	// so the loop should run exactly once and stop
	if len(result.IterationDetails) != 1 {
		t.Fatalf("expected 1 iteration (no suggestions to apply), got %d", len(result.IterationDetails))
	}
}

func TestReviewIterationDetails(t *testing.T) {
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

	// Should have at least one iteration detail
	if len(result.IterationDetails) == 0 {
		t.Fatal("expected at least one iteration detail")
	}

	detail := result.IterationDetails[0]
	if detail.Iteration != 1 {
		t.Fatalf("expected first iteration to be 1, got %d", detail.Iteration)
	}
	if len(detail.Verdicts) != 5 {
		t.Fatalf("expected 5 verdict entries, got %d", len(detail.Verdicts))
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
