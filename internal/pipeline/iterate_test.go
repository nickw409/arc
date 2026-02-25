package pipeline

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

var testBin string

func TestMain(m *testing.M) {
	// Build the mock agent binary for pipeline tests.
	tmpDir, err := os.MkdirTemp("", "pipeline-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	testBin = filepath.Join(tmpDir, "mockagent")
	cmd := exec.Command("go", "build", "-o", testBin, "../agent/testdata/mockagent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build mock agent: %v\n", err)
		os.Exit(1)
	}

	// Override the agent command name so RunState uses our mock binary.
	agentCommandName = testBin

	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupTestPlan creates a minimal plan directory structure with state.json, plan.md,
// plan.json, and returns (plansDir, cleanup).
func setupTestPlan(t *testing.T, state *arc.PhaseState) string {
	t.Helper()

	plansDir := t.TempDir()
	planName := "test-plan"
	phaseName := state.Phase

	planDir := filepath.Join(plansDir, planName)
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatalf("failed to create phase dir: %v", err)
	}

	// Write state.json
	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
		t.Fatalf("failed to write state.json: %v", err)
	}

	// Write plan.md
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Plan\nThis is a test."), 0644); err != nil {
		t.Fatalf("failed to write plan.md: %v", err)
	}

	// Write plan.json
	planMeta := &arc.PlanMeta{
		Name:         planName,
		Status:       "active",
		Phases:       []string{phaseName},
		PhaseOrder:   map[string]int{phaseName: 1},
		Dependencies: map[string][]string{},
		WorkflowType: state.WorkflowType,
	}
	metaData, err := json.MarshalIndent(planMeta, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal plan meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatalf("failed to write plan.json: %v", err)
	}

	return plansDir
}

func TestMapStateToStatusAllVariants(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"impl", "implementing"},
		{"fix", "implementing"},
		{"refactor", "implementing"},
		{"optimize", "implementing"},
		{"draft", "implementing"},
		{"research", "implementing"},
		{"characterize", "implementing"},
		{"baseline", "implementing"},
		{"analyze", "implementing"},
		{"investigate", "implementing"},
		{"regression_tests", "implementing"},
		{"qa", "qa"},
		{"qa_review", "qa_review"},
		{"test_review", "qa_review"},
		{"char_review", "qa_review"},
		{"fix_review", "qa_review"},
		{"review", "qa_review"},
		{"verify", "qa_review"},
		{"benchmark", "qa_review"},
		{"impl_review", "qa_review"},
		{"complete", "complete"},
		{"blocked", "blocked"},
		{"unknown_state", "implementing"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := MapStateToStatus(tc.input)
			if got != tc.want {
				t.Fatalf("MapStateToStatus(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMapStateToStatusEmptyString(t *testing.T) {
	got := MapStateToStatus("")
	if got != "implementing" {
		t.Fatalf("MapStateToStatus(%q) = %q, want %q", "", got, "implementing")
	}
}

func TestRunStateReturnsActionContinue(t *testing.T) {
	// Use check.adversary state which is branching (no_bugs_found -> complete, bugs_found -> impl.act).
	// Mock agent outputs a verdict of "no_bugs_found".
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "check.adversary"
	state.PhaseStatus = "check.adversary"

	plansDir := setupTestPlan(t, state)

	// Set mock agent to output a verdict.
	t.Setenv("MOCK_OUTPUT", "Some analysis\n\n## Verdict\n\nno_bugs_found\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.NextState != "complete" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "complete")
	}
	if result.Verdict != arc.VerdictNoBugsFound {
		t.Fatalf("got Verdict=%q, want %q", result.Verdict, arc.VerdictNoBugsFound)
	}
}

func TestRunStateAgentTimeout(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_SLEEP_MS", "5000")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result := RunState(ctx, testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	// Context cancelled → ActionAbort
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Err == nil {
		t.Fatal("expected error for timeout/cancel")
	}
}

func TestRunStateContextCancelled(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := RunState(ctx, testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected context.Canceled error")
	}
}

func TestRunStateLinearStateNoVerdicts(t *testing.T) {
	// "impl.act" is a linear state (next: check.adversary). No verdict extraction needed.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "impl.act"
	state.PhaseStatus = "impl.act"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "Implementation complete, no verdict needed.\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.NextState != "check.adversary" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "check.adversary")
	}
	// No verdict for linear states
	if result.Verdict != "" {
		t.Fatalf("got Verdict=%q, want empty (linear state)", result.Verdict)
	}
}

func TestRunStateNonzeroExit(t *testing.T) {
	// Agent exits with code 1 but does NOT time out.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "impl.act"
	state.PhaseStatus = "impl.act"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "some output before crash")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionRetry {
		t.Fatalf("got Action=%v, want ActionRetry", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	errStr := result.Err.Error()
	if strings.Contains(errStr, "timed out") {
		t.Fatalf("error should NOT contain 'timed out' for non-zero exit, got: %v", result.Err)
	}
}

func TestRunStateVerdictUnknown(t *testing.T) {
	// check.adversary is branching; agent outputs invalid verdict.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "check.adversary"
	state.PhaseStatus = "check.adversary"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\ninvalid_verdict_value\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionRetry {
		t.Fatalf("got Action=%v, want ActionRetry", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for unknown verdict")
	}
	errStr := result.Err.Error()
	if !strings.Contains(errStr, "not in valid set") {
		t.Fatalf("expected error containing 'not in valid set', got: %v", result.Err)
	}
}

func TestRunStateStateFileCorrupt(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phaseDir := filepath.Join(planDir, "phases", "test-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatalf("failed to create phase dir: %v", err)
	}

	// Write corrupt state.json
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupt state.json: %v", err)
	}

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

func TestRunStatePlanMDMissing(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "impl.act"
	state.PhaseStatus = "impl.act"

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phaseDir := filepath.Join(planDir, "phases", "test-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatalf("failed to create phase dir: %v", err)
	}

	// Write valid state.json
	stateData, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
		t.Fatalf("failed to write state.json: %v", err)
	}

	// Write plan.json
	planMeta := &arc.PlanMeta{
		Name:         "test-plan",
		Status:       "active",
		Phases:       []string{"test-phase"},
		PhaseOrder:   map[string]int{"test-phase": 1},
		Dependencies: map[string][]string{},
		WorkflowType: "feature",
	}
	metaData, _ := json.MarshalIndent(planMeta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatalf("failed to write plan.json: %v", err)
	}

	// Deliberately do NOT create plan.md

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for missing plan.md")
	}
	if !strings.Contains(result.Err.Error(), "plan.md") {
		t.Fatalf("expected error about plan.md, got: %v", result.Err)
	}
}

func TestRunStateTerminalStateEarlyReturn(t *testing.T) {
	// State is "complete" which is terminal — should return immediately without spawning.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "complete"
	state.PhaseStatus = "complete"

	plansDir := setupTestPlan(t, state)

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue", result.Action)
	}
	if result.NextState != "" {
		t.Fatalf("got NextState=%q, want empty for terminal state", result.NextState)
	}
}

func TestRunStateWorkflowBytesFailure(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "nonexistent-workflow-type-xyz")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for unknown workflow type")
	}
	if !strings.Contains(strings.ToLower(result.Err.Error()), "workflow") {
		t.Fatalf("expected error containing 'workflow', got: %v", result.Err)
	}
}

func TestRunStatePromptBytesFailure(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "nonexistent_state"
	state.PhaseStatus = "implementing"

	plansDir := setupTestPlan(t, state)

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort; err=%v", result.Action, result.Err)
	}
	if result.Err == nil {
		t.Fatal("expected error for missing prompt/state")
	}
}

func TestRunStateModeInstructionsAppended(t *testing.T) {
	// Use a linear state (impl.act) so no verdict extraction is needed.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "impl.act"
	state.PhaseStatus = "impl.act"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_ECHO_STDIN", "1")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:     "test-plan",
		PhaseName:    "test-phase",
		Mode:         "impl",
		Instructions: "Focus on tests",
		PlansDir:     plansDir,
	})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}

	// Verify state was updated (iteration incremented proves pipeline ran fully)
	stateFile := filepath.Join(plansDir, "test-plan", "phases", "test-phase", "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var updated arc.PhaseState
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}
	if updated.Iteration.Current != state.Iteration.Current+1 {
		t.Fatalf("expected iteration to be incremented to %d, got %d", state.Iteration.Current+1, updated.Iteration.Current)
	}
}

func TestRunStateMemoryInjection(t *testing.T) {
	// Pre-seed a memory file; verify the run succeeds (memory was read and injected).
	st := arc.NewPhaseState("test-plan", "test-phase", "feature")
	st.CurrentState = "impl.act"
	st.PhaseStatus = "impl.act"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	// Write existing memory for the "impl.act" state
	if err := WriteMemory(phaseDir, "impl.act", "previously explored: file X, file Y"); err != nil {
		t.Fatalf("setup: WriteMemory failed: %v", err)
	}

	t.Setenv("MOCK_OUTPUT", "Work done.\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}

	// Verify iteration advanced (pipeline ran without error)
	stateFile := filepath.Join(phaseDir, "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var updated arc.PhaseState
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}
	if updated.Iteration.Current != st.Iteration.Current+1 {
		t.Fatalf("expected iteration incremented, got %d", updated.Iteration.Current)
	}
}

func TestRunStateMemorySaved(t *testing.T) {
	// Agent outputs a ## Memory section; verify it gets saved to disk.
	st := arc.NewPhaseState("test-plan", "test-phase", "feature")
	st.CurrentState = "check.adversary"
	st.PhaseStatus = "check.adversary"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	t.Setenv("MOCK_OUTPUT", "Analysis complete.\n\n## Memory\nexplored src/foo.go and src/bar.go\n\n## Verdict\n\nno_bugs_found\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}

	// Verify memory file was written
	mem, err := ReadMemory(phaseDir, "check.adversary")
	if err != nil {
		t.Fatalf("ReadMemory failed: %v", err)
	}
	if !strings.Contains(mem, "explored src/foo.go") {
		t.Fatalf("expected memory to contain exploration notes, got: %q", mem)
	}
}

func TestRunStateRunOnceSkip(t *testing.T) {
	// check.adversary is run_once in the feature workflow.
	// Pre-set StateIterations to 2 to simulate a second visit (phase.go pre-increments).
	// The agent must NOT be spawned — the skip verdict is produced automatically.
	st := arc.NewPhaseState("test-plan", "test-phase", "feature")
	st.CurrentState = "check.adversary"
	st.PhaseStatus = "adversary"
	st.StateIterations = map[string]int{"check.adversary": 2}

	plansDir := setupTestPlan(t, st)

	// If the agent is spawned, it exits with code 1 — which would cause ActionRetry,
	// proving the skip logic did NOT fire.
	t.Setenv("MOCK_EXIT_CODE", "1")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue (run_once skip); err=%v", result.Action, result.Err)
	}
	if result.Verdict != arc.VerdictNoBugsFound {
		t.Fatalf("got Verdict=%q, want %q", result.Verdict, arc.VerdictNoBugsFound)
	}
	if result.NextState != "complete" {
		t.Fatalf("got NextState=%q, want complete", result.NextState)
	}
}

// withTestWorkflow installs a custom workflow loader for the duration of the test.
func withTestWorkflow(t *testing.T, wfType string, data []byte) {
	t.Helper()
	orig := workflowBytesFunc
	workflowBytesFunc = func(name string) ([]byte, error) {
		if name == wfType {
			return data, nil
		}
		return orig(name)
	}
	t.Cleanup(func() { workflowBytesFunc = orig })
}

// Ensure the pipeline types are valid by referencing them.
var _ = IterateOptions{}
var _ = (*arc.IterationResult)(nil)
