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
	"github.com/nwiley/arc/internal/resources"
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
		// Non-namespaced: returned as-is
		{"impl", "impl"},
		{"adversary", "adversary"},
		{"complete", "complete"},
		{"blocked", "blocked"},
		{"tests", "tests"},
		{"qa_review", "qa_review"},
		// Namespaced: strips prefix
		{"impl.act", "act"},
		{"check.adversary", "adversary"},
		{"regression_tests.tests", "tests"},
		{"test_review.qa_review", "qa_review"},
		{"fix_review.impl_review", "impl_review"},
		{"verify.impl_review", "impl_review"},
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
	if got != "" {
		t.Fatalf("MapStateToStatus(%q) = %q, want %q", "", got, "")
	}
}

func TestRunStateReturnsActionContinue(t *testing.T) {
	// Use audit.adversary state which is branching (no_bugs_found -> complete, bugs_found -> fix.act).
	// Mock agent outputs a verdict of "no_bugs_found".
	state := arc.NewPhaseState("test-plan", "test-phase", "audit")
	state.CurrentState = "audit.adversary"
	state.PhaseStatus = "audit.adversary"

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

func TestRunStateActBlockDoneVerdict(t *testing.T) {
	// "fix.act" in audit workflow uses act block with verdict: done → audit.adversary.
	state := arc.NewPhaseState("test-plan", "test-phase", "audit")
	state.CurrentState = "fix.act"
	state.PhaseStatus = "fix.act"

	plansDir := setupTestPlan(t, state)

	t.Setenv("MOCK_OUTPUT", "Implementation complete.\n\n## Verdict\ndone\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
	if result.NextState != "audit.adversary" {
		t.Fatalf("got NextState=%q, want %q", result.NextState, "audit.adversary")
	}
	if result.Verdict != "done" {
		t.Fatalf("got Verdict=%q, want %q", result.Verdict, "done")
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
	// audit.adversary is branching; agent outputs invalid verdict.
	state := arc.NewPhaseState("test-plan", "test-phase", "audit")
	state.CurrentState = "audit.adversary"
	state.PhaseStatus = "audit.adversary"

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

	t.Setenv("MOCK_OUTPUT", "Work done.\n\n## Verdict\ndone\n")

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
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "audit.adversary"
	st.PhaseStatus = "audit.adversary"

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
	mem, err := ReadMemory(phaseDir, "audit.adversary")
	if err != nil {
		t.Fatalf("ReadMemory failed: %v", err)
	}
	if !strings.Contains(mem, "explored src/foo.go") {
		t.Fatalf("expected memory to contain exploration notes, got: %q", mem)
	}
}

func TestRunStateRunOnceSkip(t *testing.T) {
	// _fork_0 is run_once in the feature workflow (parallel adversary fork).
	// Pre-set StateIterations to 2 and a prior verdict entry to simulate a
	// successful second visit (phase.go pre-increments, prior run completed).
	// The agent must NOT be spawned — the skip verdict is produced automatically.
	st := arc.NewPhaseState("test-plan", "test-phase", "feature")
	st.CurrentState = "_fork_0"
	st.PhaseStatus = "adversary"
	st.StateIterations = map[string]int{"_fork_0": 2}
	st.VerdictsHistory = []arc.VerdictEntry{
		{Iteration: 1, State: "_fork_0", Verdict: "bugs_found", Timestamp: "2026-01-01T00:00:00Z"},
	}

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

func TestRunStateRunOnceNoSkipWhenInterrupted(t *testing.T) {
	// If StateIterations > 1 but there is no prior verdict for _fork_0,
	// the previous visit was interrupted — the agent must run, not be skipped.
	st := arc.NewPhaseState("test-plan", "test-phase", "feature")
	st.CurrentState = "_fork_0"
	st.PhaseStatus = "adversary"
	st.StateIterations = map[string]int{"_fork_0": 2}
	// No VerdictsHistory entry for _fork_0 — simulates an interrupted run.

	plansDir := setupTestPlan(t, st)

	// Agent exits with code 1 → ActionRetry. If we see ActionRetry we know the
	// agent was actually spawned (skip did not fire).
	t.Setenv("MOCK_EXIT_CODE", "1")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionRetry {
		t.Fatalf("got Action=%v, want ActionRetry (agent must run after interrupted visit); err=%v", result.Action, result.Err)
	}
}

func TestRunStateUsesResolverWorkflow(t *testing.T) {
	// Create a custom workflow in a temp project dir
	projDir := t.TempDir()
	wfDir := filepath.Join(projDir, ".arc", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a minimal valid workflow YAML
	wfYAML := `name: test-wf
version: 1
description: Test workflow
entry_state: impl
terminal_states: [complete, blocked]
states:
  - name: impl
    description: Implement
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    description: Done
    prompt: prompts/common/complete.md
  - name: blocked
    description: Blocked
    prompt: prompts/common/blocked.md
`
	if err := os.WriteFile(filepath.Join(wfDir, "test-wf.yaml"), []byte(wfYAML), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolver := resources.NewResolver(projDir, "")

	// Create a phase in "complete" state so RunState returns early (terminal)
	state := arc.NewPhaseState("test-plan", "test-phase", "test-wf")
	state.CurrentState = "complete"
	state.PhaseStatus = "complete"

	plansDir := setupTestPlan(t, state)

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		Resolver:  resolver,
	})

	// Should succeed — terminal state detected after loading the custom workflow
	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunStateNilResolverFallsBackToEmbedded(t *testing.T) {
	// Nil resolver should fall back to embedded workflows (e.g. "feature")
	state := arc.NewPhaseState("test-plan", "test-phase", "feature")
	state.CurrentState = "complete"
	state.PhaseStatus = "complete"

	plansDir := setupTestPlan(t, state)

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
		Resolver:  nil, // explicitly nil
	})

	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunStateAppendsAgentLifecycleToHistory(t *testing.T) {
	// Linear state: impl.act → check.adversary. Agent exits 0.
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "fix.act"
	st.PhaseStatus = "fix.act"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	t.Setenv("MOCK_OUTPUT", "Implementation complete.\n\n## Verdict\ndone\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})
	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}

	history, err := os.ReadFile(filepath.Join(phaseDir, "history.md"))
	if err != nil {
		t.Fatalf("failed to read history.md: %v", err)
	}
	histStr := string(history)
	if !strings.Contains(histStr, "[fix.act] agent pid=") {
		t.Fatalf("history.md missing lifecycle entry, got:\n%s", histStr)
	}
	if !strings.Contains(histStr, "exit=0") {
		t.Fatalf("history.md missing exit=0, got:\n%s", histStr)
	}
	if !strings.Contains(histStr, "duration=") {
		t.Fatalf("history.md missing duration, got:\n%s", histStr)
	}
}

func TestRunStateAppendsStderrSnippetOnNonZeroExit(t *testing.T) {
	// Branching state: audit.adversary exits 1 with stderr.
	// Exit 1 + no valid verdict → ActionRetry, but history should contain stderr.
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "audit.adversary"
	st.PhaseStatus = "audit.adversary"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_STDERR", "connection refused")
	t.Setenv("MOCK_OUTPUT", "## Verdict\n\nno_bugs_found\n") // verdict present despite non-zero exit

	_ = RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	history, err := os.ReadFile(filepath.Join(phaseDir, "history.md"))
	if err != nil {
		t.Fatalf("failed to read history.md: %v", err)
	}
	histStr := string(history)
	if !strings.Contains(histStr, "stderr: connection refused") {
		t.Fatalf("history.md missing stderr snippet, got:\n%s", histStr)
	}
}

func TestRunStateTruncatesLongStderrInHistory(t *testing.T) {
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "fix.act"
	st.PhaseStatus = "fix.act"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	longStderr := strings.Repeat("x", 600)
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_STDERR", longStderr)
	t.Setenv("MOCK_OUTPUT", "some output")

	_ = RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	history, err := os.ReadFile(filepath.Join(phaseDir, "history.md"))
	if err != nil {
		t.Fatalf("failed to read history.md: %v", err)
	}
	histStr := string(history)
	// Find the stderr line
	for _, line := range strings.Split(histStr, "\n") {
		if strings.Contains(line, "stderr:") {
			// Extract the snippet after "stderr: "
			idx := strings.Index(line, "stderr: ")
			if idx < 0 {
				continue
			}
			snippet := line[idx+len("stderr: "):]
			if len(snippet) != 503 { // 500 + "..."
				t.Fatalf("stderr snippet length = %d, want 503 (500 + '...')", len(snippet))
			}
			if !strings.HasSuffix(snippet, "...") {
				t.Fatalf("stderr snippet should end with '...', got: %q", snippet[len(snippet)-10:])
			}
			return
		}
	}
	t.Fatal("history.md missing stderr line")
}

func TestRunStateNoStderrAppendOnZeroExit(t *testing.T) {
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "fix.act"
	st.PhaseStatus = "fix.act"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	t.Setenv("MOCK_STDERR", "some warning")
	t.Setenv("MOCK_OUTPUT", "done\n\n## Verdict\ndone\n")

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})
	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}

	history, err := os.ReadFile(filepath.Join(phaseDir, "history.md"))
	if err != nil {
		t.Fatalf("failed to read history.md: %v", err)
	}
	if strings.Contains(string(history), "stderr:") {
		t.Fatalf("history.md should NOT contain stderr line on exit=0, got:\n%s", string(history))
	}
}

func TestRunStateNoStderrAppendWhenEmpty(t *testing.T) {
	st := arc.NewPhaseState("test-plan", "test-phase", "audit")
	st.CurrentState = "fix.act"
	st.PhaseStatus = "fix.act"

	plansDir := setupTestPlan(t, st)
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "test-phase")

	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "some output")
	// No MOCK_STDERR set

	_ = RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	history, err := os.ReadFile(filepath.Join(phaseDir, "history.md"))
	if err != nil {
		t.Fatalf("failed to read history.md: %v", err)
	}
	if strings.Contains(string(history), "stderr:") {
		t.Fatalf("history.md should NOT contain stderr line when stderr is empty, got:\n%s", string(history))
	}
}

func TestRunStateCustomWorkflowFromPlanDir(t *testing.T) {
	// WorkflowType "custom" should resolve workflow.yaml from the plan directory
	st := arc.NewPhaseState("test-plan", "test-phase", "custom")
	st.CurrentState = "complete"
	st.PhaseStatus = "complete"

	plansDir := setupTestPlan(t, st)

	// Write workflow.yaml to the plan directory
	wfYAML := `name: custom-inline
version: 1
description: Custom inline workflow
entry_state: impl
terminal_states: [complete, blocked]
states:
  - name: impl
    description: Implement
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    description: Done
    prompt: prompts/common/complete.md
  - name: blocked
    description: Blocked
    prompt: prompts/common/blocked.md
`
	planDir := filepath.Join(plansDir, "test-plan")
	if err := os.WriteFile(filepath.Join(planDir, "workflow.yaml"), []byte(wfYAML), 0644); err != nil {
		t.Fatalf("failed to write workflow.yaml: %v", err)
	}

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	// Should succeed — terminal state detected after loading custom workflow
	if result.Action != arc.ActionContinue {
		t.Fatalf("got Action=%v, want ActionContinue; err=%v", result.Action, result.Err)
	}
}

func TestRunStateCustomWorkflowMissingFile(t *testing.T) {
	// WorkflowType "custom" with no workflow.yaml should fail
	st := arc.NewPhaseState("test-plan", "test-phase", "custom")
	st.CurrentState = "impl"
	st.PhaseStatus = "impl"

	plansDir := setupTestPlan(t, st)
	// Do NOT write workflow.yaml

	result := RunState(context.Background(), testLogger(), IterateOptions{
		PlanName:  "test-plan",
		PhaseName: "test-phase",
		PlansDir:  plansDir,
	})

	if result.Action != arc.ActionAbort {
		t.Fatalf("got Action=%v, want ActionAbort", result.Action)
	}
	if result.Err == nil {
		t.Fatal("expected error for missing workflow.yaml")
	}
}

// Ensure the pipeline types are valid by referencing them.
var _ = IterateOptions{}
var _ = (*arc.IterationResult)(nil)
