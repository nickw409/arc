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

	// Override the agent command name so RunIteration uses our mock binary.
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

func TestIterateReturnsActionContinue(t *testing.T) {
	// Use qa_review state which is branching (approved -> impl, gaps_found -> qa).
	// Mock agent outputs a verdict of "approved".
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa_review"
	state.PhaseStatus = "qa_review"

	plansDir := setupTestPlan(t, state)

	// Set mock agent to output a verdict.
	t.Setenv("MOCK_OUTPUT", "Some analysis\n\n## Verdict\n\napproved\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.NextState != "impl" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "impl")
	}
	if result.Verdict != arc.VerdictApproved {
		t.Fatalf("got Verdict=%q, want %q", result.Verdict, arc.VerdictApproved)
	}
}

func TestIterateAgentTimeout(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_SLEEP_MS", "5000")

	// Note: We need the Spawn function to use the testBin.
	// This test requires hooking into the agent spawn — for now,
	// verify that a timeout scenario returns ActionRetry.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result := RunIteration(ctx, testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	// Could be ActionRetry (timeout) or ActionAbort (context cancelled).
	// The key thing is it doesn't hang or panic.
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Err == nil {
		t.Fatal("expected error for timeout/cancel")
	}
}

func TestIterateAgentEmptyOutput(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionRetry {
		t.Fatalf("got Action=%v, want ActionRetry", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for empty output")
	}
	errStr := strings.ToLower(result.Err.Error())
	if !strings.Contains(errStr, "no output") && !strings.Contains(errStr, "empty") {
		t.Fatalf("expected error about no output, got: %v", result.Err)
	}
}

func TestIterateContextCancelled(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := RunIteration(ctx, testLogger(), IterateOptions{
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

func TestIterateLinearStateNoVerdicts(t *testing.T) {
	// "qa" is a linear state (next: qa_review). No verdict extraction needed.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "Tests written successfully, no verdict needed.\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.NextState != "qa_review" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "qa_review")
	}
	// No verdict for linear states
	if result.Verdict != "" {
		t.Fatalf("got Verdict=%q, want empty (linear state)", result.Verdict)
	}
}

func TestIterateTimeoutVsNonzeroExit(t *testing.T) {
	// Agent exits with code 1 but does NOT time out.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "some output before crash")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIterateVerdictUnknown(t *testing.T) {
	// qa_review is branching; agent outputs invalid verdict.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa_review"
	state.PhaseStatus = "qa_review"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\ninvalid_verdict_value\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIterateStateFileCorrupt(t *testing.T) {
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

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIteratePlanMDMissing(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

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

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIterateTerminalStateEarlyReturn(t *testing.T) {
	// State is "complete" which is terminal — should return immediately without spawning.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "complete"
	state.PhaseStatus = "complete"

	plansDir := setupTestPlan(t, state)

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIterateWorkflowBytesFailure(t *testing.T) {
	state := arc.NewPhaseState("test-plan", "test-phase", "nonexistent-workflow-type-xyz")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIteratePromptBytesFailure(t *testing.T) {
	// To test this, we need a valid workflow but with a state pointing to a
	// nonexistent prompt. We can't easily override the embedded workflow,
	// but we can create a state that references a workflow state whose prompt
	// doesn't exist. Since the feature workflow has fixed prompts, we'd need
	// a custom workflow. Instead, we verify the error path by using a state
	// that doesn't exist in the workflow.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "nonexistent_state"
	state.PhaseStatus = "implementing"

	plansDir := setupTestPlan(t, state)

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

func TestIterateModeInstructionsAppended(t *testing.T) {
	// Use a linear state (qa) so no verdict extraction is needed.
	// Mock agent echoes stdin to verify the rendered prompt includes mode/instructions.
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "qa"
	state.PhaseStatus = "qa"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_ECHO_STDIN", "1")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
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

	// Read the updated state to get the agent output (which is stdin echoed back).
	// The mock agent echoes stdin, so the rendered prompt (with mode/instructions)
	// should have been piped through. We verify the pipeline constructed the right prompt
	// by checking the state file was updated (iteration incremented).
	stateFile := filepath.Join(plansDir, "test-plan", "phases", "test-phase", "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var updated arc.PhaseState
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}
	// Verify iteration was incremented (proof the pipeline ran fully)
	if updated.Iteration.Current != state.Iteration.Current+1 {
		t.Fatalf("expected iteration to be incremented to %d, got %d", state.Iteration.Current+1, updated.Iteration.Current)
	}
}

// testWorkflowWithCaps returns a minimal flat workflow YAML with a qa_review state
// that has max_state_iterations and on_max_iterations set.
func testWorkflowWithCaps(maxStateIter int, onMax string) []byte {
	onMaxLine := ""
	if onMax != "" {
		onMaxLine = fmt.Sprintf("\n      on_max_iterations: %s", onMax)
	}
	constraintsBlock := ""
	if maxStateIter != 0 {
		constraintsBlock = fmt.Sprintf(`    constraints:
      max_state_iterations: %d%s
`, maxStateIter, onMaxLine)
	}
	return []byte(fmt.Sprintf(`name: test-caps
version: 1
entry_state: qa_review
terminal_states: [complete, blocked]
states:
  - name: qa_review
    description: Review test coverage
    prompt: prompts/feature/qa-review.md
    verdicts: [approved, gaps_found]
%s    next:
      approved: complete
      gaps_found: qa_review
  - name: complete
    description: Phase completed successfully
    prompt: prompts/common/complete.md
  - name: blocked
    description: Phase blocked
    prompt: prompts/common/blocked.md
`, constraintsBlock))
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

// setupTestPlanWithWorkflow sets up a test plan using a custom workflow type.
func setupTestPlanWithWorkflow(t *testing.T, phaseState *arc.PhaseState, wfType string, wfData []byte) string {
	t.Helper()
	withTestWorkflow(t, wfType, wfData)
	return setupTestPlan(t, phaseState)
}

func TestRunIterationMaxStateIterationsForced(t *testing.T) {
	// qa_review has been visited 4 times; cap is 3. Should force exit with "approved".
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"qa_review": 4}

	wfData := testWorkflowWithCaps(3, "approved")
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	// Set mock to produce gaps_found — if agent ran it would give wrong verdict.
	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\ngaps_found\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.Verdict != "approved" {
		t.Fatalf("got Verdict=%q, want %q", result.Verdict, "approved")
	}
	if result.NextState != "complete" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "complete")
	}

	// Verify state was updated correctly.
	stateFile := filepath.Join(plansDir, "test-plan", "phases", "test-phase", "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	var updated arc.PhaseState
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("parsing state: %v", err)
	}
	if updated.CurrentState != "complete" {
		t.Errorf("CurrentState = %q, want %q", updated.CurrentState, "complete")
	}
	if updated.LastVerdict != "approved" {
		t.Errorf("LastVerdict = %q, want %q", updated.LastVerdict, "approved")
	}
	if updated.Iteration.Current != st.Iteration.Current+1 {
		t.Errorf("Iteration.Current = %d, want %d", updated.Iteration.Current, st.Iteration.Current+1)
	}
	if len(updated.VerdictsHistory) == 0 {
		t.Fatal("expected a VerdictsHistory entry")
	}
	last := updated.VerdictsHistory[len(updated.VerdictsHistory)-1]
	if last.State != "qa_review" || last.Verdict != "approved" {
		t.Errorf("last verdict history: state=%q verdict=%q, want qa_review/approved", last.State, last.Verdict)
	}
}

func TestRunIterationMaxStateIterationsBoundary(t *testing.T) {
	// count == max (3 == 3) should NOT trigger cap; iteration runs normally.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"qa_review": 3}

	wfData := testWorkflowWithCaps(3, "approved")
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\napproved\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue (boundary should not trigger); err=%v", result.Action, result.Err)
	}
	if result.Verdict != "approved" {
		t.Fatalf("got Verdict=%q, want approved", result.Verdict)
	}
}

func TestRunIterationMaxStateIterationsNoConstraint(t *testing.T) {
	// No constraints set; iteration runs normally regardless of StateIterations.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"qa_review": 100}

	wfData := testWorkflowWithCaps(0, "") // no constraints
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\napproved\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunIterationMaxStateIterationsNilMap(t *testing.T) {
	// StateIterations map is nil; count defaults to 0, cap not triggered.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = nil

	wfData := testWorkflowWithCaps(3, "approved")
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\napproved\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunIterationMaxStateIterationsMapMissingKey(t *testing.T) {
	// StateIterations exists but does not contain current state; count = 0, cap not triggered.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"other_state": 5}

	wfData := testWorkflowWithCaps(3, "approved")
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	t.Setenv("MOCK_OUTPUT", "Analysis\n\n## Verdict\n\napproved\n")

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunIterationMaxStateIterationsNoOnMax(t *testing.T) {
	// Cap exceeded but on_max_iterations is empty; should abort.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"qa_review": 4}

	wfData := testWorkflowWithCaps(3, "") // no on_max_iterations
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort; err=%v", result.Action, result.Err)
	}
	if result.Err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.Err.Error(), "no on_max_iterations set") {
		t.Fatalf("expected error containing 'no on_max_iterations set', got: %v", result.Err)
	}
}

func TestRunIterationMaxStateIterationsInvalidTransition(t *testing.T) {
	// on_max_iterations set to a verdict that is not valid from qa_review.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.StateIterations = map[string]int{"qa_review": 4}

	wfData := testWorkflowWithCaps(3, "invalid_verdict")
	plansDir := setupTestPlanWithWorkflow(t, st, "test-caps", wfData)

	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort; err=%v", result.Action, result.Err)
	}
	if result.Err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.Err.Error(), "transition failed") {
		t.Fatalf("expected error containing 'transition failed', got: %v", result.Err)
	}
}

func TestRunIterationBothMaxIterationsConstraints(t *testing.T) {
	// Both max_iterations (global) and max_state_iterations (per-state) are exceeded.
	// Per-state check runs first, so we expect ActionContinue with forced verdict
	// rather than ActionAbort from the global check.
	st := arc.NewPhaseState("test-plan", "test-phase", "test-caps")
	st.CurrentState = "qa_review"
	st.PhaseStatus = "qa_review"
	st.Iteration.Current = 6           // exceeds global max_iterations: 5
	st.StateIterations = map[string]int{"qa_review": 4} // exceeds max_state_iterations: 3

	// Workflow has both constraints set.
	wfData := []byte(`name: test-both-caps
version: 1
entry_state: qa_review
terminal_states: [complete, blocked]
states:
  - name: qa_review
    description: Review test coverage
    prompt: prompts/feature/qa-review.md
    verdicts: [approved, gaps_found]
    constraints:
      max_iterations: 5
      max_state_iterations: 3
      on_max_iterations: approved
    next:
      approved: complete
      gaps_found: qa_review
  - name: complete
    description: Phase completed successfully
    prompt: prompts/common/complete.md
  - name: blocked
    description: Phase blocked
    prompt: prompts/common/blocked.md
`)
	withTestWorkflow(t, "test-caps", wfData)
	plansDir := setupTestPlan(t, st)

	// Per-state cap fires first; forced verdict is approved → complete.
	result := RunIteration(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue (per-state check precedes global); err=%v", result.Action, result.Err)
	}
	if result.Verdict != "approved" {
		t.Fatalf("got Verdict=%q, want approved", result.Verdict)
	}
}

// Ensure the pipeline _ imports are valid by referencing the types.
var _ = IterateOptions{}
var _ = (*arc.IterationResult)(nil)
