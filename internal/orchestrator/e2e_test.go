package orchestrator

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
	"github.com/nwiley/arc/internal/pipeline"
	"github.com/nwiley/arc/internal/state"
)

var mockAgentBin string

func TestMain(m *testing.M) {
	// Build the mock agent binary once for all tests.
	tmp, err := os.MkdirTemp("", "arc-e2e-mock-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "mockagent")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/mockagent")
	cmd.Dir = filepath.Join("..", "agent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build mockagent: " + err.Error())
	}
	mockAgentBin = bin

	// Override all command name vars so all agent spawns use the mock.
	pipeline.SetAgentCommandNameForTest(bin)
	judgeCommandName = bin
	directAgentCmd = bin

	os.Exit(m.Run())
}

// initE2EGitRepo creates an isolated git repository in a temp directory.
func initE2EGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init cmd %v failed: %v\n%s", args, err, out)
		}
	}

	// Create an initial commit so the repo is not empty
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	return dir
}

// setupE2E creates a plan directory with phases and returns (plansDir, scriptDir, projectDir).
// projectDir is an isolated git repository that prevents commits from affecting the real repo.
func setupE2E(t *testing.T, planName string, phases []string, workflowType string) (string, string, string) {
	t.Helper()

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, planName)

	for _, phase := range phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}

		ps := arc.NewPhaseState(planName, phase, workflowType)
		writeState(t, phaseDir, ps)

		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Plan\nTest."), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Write plan.json
	meta := arc.NewPlanMeta(planName, workflowType, phases)
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create script directory for mock responses
	scriptDir := t.TempDir()
	t.Setenv("MOCK_SCRIPT_DIR", scriptDir)

	// Create isolated git repo for commits
	projectDir := initE2EGitRepo(t)

	return plansDir, scriptDir, projectDir
}

func writeState(t *testing.T, phaseDir string, ps *arc.PhaseState) {
	t.Helper()
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readState(t *testing.T, plansDir, planName, phaseName string) *arc.PhaseState {
	t.Helper()
	sf := state.NewStateFile(filepath.Join(plansDir, planName, "phases", phaseName, "state.json"))
	ps, err := sf.Read()
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func readCallCount(t *testing.T, scriptDir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scriptDir, ".call_count"))
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &n)
	return n
}

func e2eLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestE2EHappyPath exercises the full happy path:
// impl.act → check.adversary(no_bugs_found) → complete
func TestE2EHappyPath(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-happy", []string{"core"}, "feature")

	// Call 0: impl.act (linear, no verdict needed)
	writeScript(t, scriptDir, 0, "Implementation complete.")
	// Call 1: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 1, "No bugs found.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-happy",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-happy", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected current_state=complete, got %q", ps.CurrentState)
	}
	if ps.PhaseStatus != "complete" {
		t.Fatalf("expected phase_status=complete, got %q", ps.PhaseStatus)
	}
	// 2 agent calls = 2 iterations
	if ps.Iteration.Current != 2 {
		t.Fatalf("expected iteration.current=2, got %d", ps.Iteration.Current)
	}
	if ps.GlobalIterations != 2 {
		t.Fatalf("expected global_iterations=2, got %d", ps.GlobalIterations)
	}
	if ps.LastVerdict != "no_bugs_found" {
		t.Fatalf("expected last_verdict=no_bugs_found, got %q", ps.LastVerdict)
	}
	// Exactly 1 verdict: check.adversary→no_bugs_found
	if len(ps.VerdictsHistory) != 1 {
		t.Fatalf("expected 1 verdict in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "no_bugs_found" || ps.VerdictsHistory[0].State != "check.adversary" {
		t.Fatalf("unexpected first verdict: %+v", ps.VerdictsHistory[0])
	}
	// Verify exactly 2 mock calls were made
	if n := readCallCount(t, scriptDir); n != 2 {
		t.Fatalf("expected 2 mock agent calls, got %d", n)
	}
}

// TestE2EBugsFoundLoop exercises the adversary bugs_found loop:
// impl.act → check.adversary(bugs_found) → impl.act → check.adversary(no_bugs_found) → complete
func TestE2EBugsFoundLoop(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-gaps", []string{"core"}, "feature")

	// Call 0: impl.act
	writeScript(t, scriptDir, 0, "Implementation written.")
	// Call 1: check.adversary → bugs_found
	writeScript(t, scriptDir, 1, "Bugs found.\n\n## Verdict\nbugs_found")
	// Call 2: impl.act (re-run to fix bugs)
	writeScript(t, scriptDir, 2, "Bugs fixed.")
	// Call 3: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 3, "All clean.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-gaps",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-gaps", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete, got %q", ps.CurrentState)
	}
	if ps.PhaseStatus != "complete" {
		t.Fatalf("expected phase_status=complete, got %q", ps.PhaseStatus)
	}
	if ps.Iteration.Current != 4 {
		t.Fatalf("expected iteration.current=4, got %d", ps.Iteration.Current)
	}
	if ps.GlobalIterations != 4 {
		t.Fatalf("expected global_iterations=4, got %d", ps.GlobalIterations)
	}
	// 2 verdicts: bugs_found, no_bugs_found (both from check.adversary)
	if len(ps.VerdictsHistory) != 2 {
		t.Fatalf("expected 2 verdicts in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "bugs_found" || ps.VerdictsHistory[0].State != "check.adversary" {
		t.Fatalf("expected first verdict bugs_found@check.adversary, got %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "no_bugs_found" || ps.VerdictsHistory[1].State != "check.adversary" {
		t.Fatalf("expected second verdict no_bugs_found@check.adversary, got %+v", ps.VerdictsHistory[1])
	}
	// run_once: check.adversary is skipped on second visit, so only 3 actual agent calls.
	if n := readCallCount(t, scriptDir); n != 3 {
		t.Fatalf("expected 3 mock agent calls, got %d", n)
	}
}

// TestE2EStateIterationTracking verifies per-state iteration counts persist across re-entry.
// Flow: impl.act(1) → check.adversary(bugs_found) → impl.act(2) → check.adversary(no_bugs_found) → complete
// impl.act should be at 2 when re-entered after bugs_found.
func TestE2EStateIterationTracking(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-stateiter", []string{"core"}, "feature")

	// Call 0: impl.act
	writeScript(t, scriptDir, 0, "Implementation written.")
	// Call 1: check.adversary → bugs_found
	writeScript(t, scriptDir, 1, "Bugs found.\n\n## Verdict\nbugs_found")
	// Call 2: impl.act (re-run to fix bugs)
	writeScript(t, scriptDir, 2, "Bugs fixed.")
	// Call 3: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 3, "All clean.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-stateiter",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-stateiter", "core")

	// impl.act ran twice (once initially, once after bugs_found)
	if ps.StateIterations["impl.act"] != 2 {
		t.Fatalf("expected impl.act state_iterations=2, got %d", ps.StateIterations["impl.act"])
	}
	// check.adversary ran twice (bugs_found then no_bugs_found)
	if ps.StateIterations["check.adversary"] != 2 {
		t.Fatalf("expected check.adversary state_iterations=2, got %d", ps.StateIterations["check.adversary"])
	}
}

// TestE2ERunOnceAdversarySkip verifies that run_once on check.adversary causes it to be
// automatically skipped on the second visit, even if its script would return bugs_found.
// Flow: impl.act → check.adversary(bugs_found) → impl.act → check.adversary(auto-skip→no_bugs_found) → complete
func TestE2ERunOnceAdversarySkip(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-concerns", []string{"core"}, "feature")

	// Call 0: impl.act
	writeScript(t, scriptDir, 0, "First implementation attempt.")
	// Call 1: check.adversary → bugs_found (first and only real adversary run)
	writeScript(t, scriptDir, 1, "Bugs found.\n\n## Verdict\nbugs_found")
	// Call 2: impl.act (fix bugs)
	writeScript(t, scriptDir, 2, "Fixed the bugs.")
	// Script 3 would be check.adversary again, but run_once skips it.
	// Write it as bugs_found to prove it is never consulted.
	writeScript(t, scriptDir, 3, "More bugs.\n\n## Verdict\nbugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-concerns",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-concerns", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete, got %q", ps.CurrentState)
	}
	if ps.Iteration.Current != 4 {
		t.Fatalf("expected iteration.current=4, got %d", ps.Iteration.Current)
	}
	// 2 verdicts: bugs_found (real), no_bugs_found (auto-skip)
	if len(ps.VerdictsHistory) != 2 {
		t.Fatalf("expected 2 verdicts in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "bugs_found" || ps.VerdictsHistory[0].State != "check.adversary" {
		t.Fatalf("expected first verdict bugs_found@check.adversary, got %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "no_bugs_found" || ps.VerdictsHistory[1].State != "check.adversary" {
		t.Fatalf("expected second verdict no_bugs_found@check.adversary (auto-skip), got %+v", ps.VerdictsHistory[1])
	}
	// Only 3 actual agent calls — script 3 is never used.
	if n := readCallCount(t, scriptDir); n != 3 {
		t.Fatalf("expected 3 mock agent calls (run_once skip), got %d", n)
	}
}

// TestE2EDisputeApproved exercises the dispute-approved flow:
// JudgeDispute → APPROVE_DISPUTE → fix iteration → impl.act continues → check.adversary(no_bugs_found) → complete
func TestE2EDisputeApproved(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-dispute-ok", []string{"core"}, "feature")

	// Pre-seed state to disputed status in impl.act state
	phaseDir := filepath.Join(plansDir, "e2e-dispute-ok", "phases", "core")
	ps := arc.NewPhaseState("e2e-dispute-ok", "core", "feature")
	ps.CurrentState = "impl.act"
	ps.PhaseStatus = "disputed"
	ps.Disputes = []arc.Dispute{
		{TestName: "TestFoo", Reason: "test is wrong"},
	}
	writeState(t, phaseDir, ps)

	// Call 0: JudgeDispute → approve
	writeScript(t, scriptDir, 0, "APPROVE_DISPUTE: test wrong")
	// Call 1: fix iteration (linear impl.act state, agent does fix work)
	writeScript(t, scriptDir, 1, "Fixed the test.")
	// Call 2: impl.act continues (linear → check.adversary)
	writeScript(t, scriptDir, 2, "Implementation done.")
	// Call 3: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 3, "No bugs found.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-dispute-ok",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps = readState(t, plansDir, "e2e-dispute-ok", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete, got %q", ps.CurrentState)
	}
	if ps.PhaseStatus != "complete" {
		t.Fatalf("expected phase_status=complete, got %q", ps.PhaseStatus)
	}
	if len(ps.Disputes) != 0 {
		t.Fatalf("expected disputes cleared, got %d", len(ps.Disputes))
	}
	if len(ps.LastClearedDisputes) != 1 {
		t.Fatalf("expected 1 last_cleared_dispute, got %d", len(ps.LastClearedDisputes))
	}
	if ps.LastClearedDisputes[0].TestName != "TestFoo" {
		t.Fatalf("expected cleared dispute for TestFoo, got %q", ps.LastClearedDisputes[0].TestName)
	}
}

// TestE2EDisputeRejected exercises the dispute-rejected flow:
// JudgeDispute → REJECT_DISPUTE → impl.act continues → check.adversary(no_bugs_found) → complete
func TestE2EDisputeRejected(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-dispute-no", []string{"core"}, "feature")

	// Pre-seed state to disputed status in impl.act state
	phaseDir := filepath.Join(plansDir, "e2e-dispute-no", "phases", "core")
	ps := arc.NewPhaseState("e2e-dispute-no", "core", "feature")
	ps.CurrentState = "impl.act"
	ps.PhaseStatus = "disputed"
	ps.Disputes = []arc.Dispute{
		{TestName: "TestBar", Reason: "impl is wrong"},
	}
	writeState(t, phaseDir, ps)

	// Call 0: JudgeDispute → reject
	writeScript(t, scriptDir, 0, "REJECT_DISPUTE: impl wrong")
	// Call 1: impl.act continues (linear → check.adversary)
	writeScript(t, scriptDir, 1, "Implementation fixed.")
	// Call 2: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 2, "No bugs found.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-dispute-no",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps = readState(t, plansDir, "e2e-dispute-no", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete, got %q", ps.CurrentState)
	}
	if ps.PhaseStatus != "complete" {
		t.Fatalf("expected phase_status=complete, got %q", ps.PhaseStatus)
	}
	if len(ps.Disputes) != 0 {
		t.Fatalf("expected disputes cleared, got %d", len(ps.Disputes))
	}
	// RejectDispute copies disputes to LastClearedDisputes
	if len(ps.LastClearedDisputes) != 1 {
		t.Fatalf("expected 1 last_cleared_dispute, got %d", len(ps.LastClearedDisputes))
	}
	if ps.LastClearedDisputes[0].TestName != "TestBar" {
		t.Fatalf("expected cleared dispute for TestBar, got %q", ps.LastClearedDisputes[0].TestName)
	}
	if n := readCallCount(t, scriptDir); n != 3 {
		t.Fatalf("expected 3 mock agent calls, got %d", n)
	}
}

// TestE2EMultiPhase exercises multi-phase orchestration with dependencies.
// Two phases: "core" (no deps) → "api" (depends on "core"). Run via Launch().
func TestE2EMultiPhase(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-multi", []string{"core", "api"}, "feature")

	// Phase "core": calls 0-1 (happy path: impl.act → check.adversary)
	writeScript(t, scriptDir, 0, "Core implementation.")
	writeScript(t, scriptDir, 1, "No bugs.\n\n## Verdict\nno_bugs_found")

	// Phase "api": calls 2-3 (happy path)
	writeScript(t, scriptDir, 2, "API implementation.")
	writeScript(t, scriptDir, 3, "No bugs.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := Launch(ctx, LaunchOptions{
		PlanName:   "e2e-multi",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if result.Status != "complete" {
		t.Fatalf("expected result status complete, got %q", result.Status)
	}

	// Verify both phases complete
	coreState := readState(t, plansDir, "e2e-multi", "core")
	if coreState.CurrentState != "complete" {
		t.Fatalf("core: expected complete, got %q", coreState.CurrentState)
	}
	if coreState.PhaseStatus != "complete" {
		t.Fatalf("core: expected phase_status=complete, got %q", coreState.PhaseStatus)
	}
	apiState := readState(t, plansDir, "e2e-multi", "api")
	if apiState.CurrentState != "complete" {
		t.Fatalf("api: expected complete, got %q", apiState.CurrentState)
	}
	if apiState.PhaseStatus != "complete" {
		t.Fatalf("api: expected phase_status=complete, got %q", apiState.PhaseStatus)
	}

	// Verify completion report generated with meaningful content
	reportPath := filepath.Join(plansDir, "e2e-multi", "COMPLETION_REPORT.md")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected COMPLETION_REPORT.md to exist: %v", err)
	}
	report := string(reportData)
	if !strings.Contains(report, "e2e-multi") {
		t.Fatal("completion report missing plan name")
	}
	if !strings.Contains(report, "2/2 complete") {
		t.Fatal("completion report missing correct phase count")
	}
	if !strings.Contains(report, "core") || !strings.Contains(report, "api") {
		t.Fatal("completion report missing phase names")
	}

	// Verify all 4 mock calls were consumed
	if n := readCallCount(t, scriptDir); n != 4 {
		t.Fatalf("expected 4 mock agent calls, got %d", n)
	}
}


// TestE2EBlockedPhaseReturnsError verifies that RunPhase returns an error
// immediately when a phase has phase_status=blocked.
func TestE2EBlockedPhaseReturnsError(t *testing.T) {
	plansDir, scriptDir, _ := setupE2E(t, "e2e-preblocked", []string{"core"}, "feature")

	phaseDir := filepath.Join(plansDir, "e2e-preblocked", "phases", "core")
	ps := arc.NewPhaseState("e2e-preblocked", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "blocked"
	reason := "permanently stuck"
	ps.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
	writeState(t, phaseDir, ps)

	// No mock scripts needed — should bail before calling any agent
	writeScript(t, scriptDir, 0, "should not be called")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-preblocked",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	})
	if err == nil {
		t.Fatal("expected RunPhase to return error for blocked phase")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected error to mention 'blocked', got: %v", err)
	}
	// Verify no mock calls were made
	if n := readCallCount(t, scriptDir); n != 0 {
		t.Fatalf("expected 0 mock agent calls for blocked phase, got %d", n)
	}
}

// TestE2EContextCancellation verifies that RunPhase respects context cancellation
// and returns promptly without hanging.
func TestE2EContextCancellation(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-cancel", []string{"core"}, "feature")

	// Call 0: impl.act (this will succeed)
	writeScript(t, scriptDir, 0, "Implementation written.")
	// No further scripts — the phase will try to proceed to check.adversary and
	// call the mock agent again. With MOCK_SCRIPT_DIR, call_1.txt is missing
	// so it falls through to MOCK_OUTPUT (empty), yielding empty output which
	// triggers ActionRetry in the loop. We cancel the context before that.

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-cancel",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected RunPhase to return error on cancellation")
	}
	// Should complete reasonably quickly (within a few seconds), not hang
	if elapsed > 10*time.Second {
		t.Fatalf("RunPhase took too long to respond to cancellation: %v", elapsed)
	}
}

// TestE2EInvalidVerdictRetries verifies that an agent returning output without
// a valid verdict section causes a retry (not a crash).
func TestE2EInvalidVerdictRetries(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-badverdict", []string{"core"}, "feature")

	// Call 0: impl.act (linear, succeeds)
	writeScript(t, scriptDir, 0, "Implementation written.")
	// Call 1: check.adversary — missing ## Verdict header → triggers retry
	writeScript(t, scriptDir, 1, "This review has no verdict section at all.")
	// Call 2: check.adversary retry — now with valid verdict
	writeScript(t, scriptDir, 2, "Better review.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-badverdict",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-badverdict", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete after retry, got %q", ps.CurrentState)
	}
	// 3 calls total: impl.act, bad check.adversary, good check.adversary
	if n := readCallCount(t, scriptDir); n != 3 {
		t.Fatalf("expected 3 mock agent calls (including retry), got %d", n)
	}
}

// TestE2EEmptyOutputRetries verifies that an agent returning empty output
// triggers a retry rather than crashing.
func TestE2EEmptyOutputRetries(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-empty", []string{"core"}, "feature")

	// Call 0: impl.act — empty output → retry
	writeScript(t, scriptDir, 0, "")
	// Call 1: impl.act — retry with real output
	writeScript(t, scriptDir, 1, "Implementation written.")
	// Call 2: check.adversary → no_bugs_found
	writeScript(t, scriptDir, 2, "No bugs.\n\n## Verdict\nno_bugs_found")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-empty",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-empty", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete after retry, got %q", ps.CurrentState)
	}
	// 3 calls: empty impl.act, real impl.act, check.adversary
	if n := readCallCount(t, scriptDir); n != 3 {
		t.Fatalf("expected 3 mock agent calls (including retry), got %d", n)
	}
}

// TestE2ENonZeroExitRetries verifies that an agent exiting with non-zero
// status triggers a retry.
func TestE2ENonZeroExitRetries(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-exit", []string{"core"}, "feature")
	t.Setenv("MOCK_EXIT_CODE", "1")

	// Write script files for multiple calls so the mock always has
	// output. On slow CI runners the mock process can be killed before
	// writing .call_count when using a tight timeout, so give plenty of
	// room for at least 2 spawns to complete.
	for i := 0; i < 10; i++ {
		writeScript(t, scriptDir, i, "fail")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:   "e2e-exit",
		PhaseName:  "core",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	// Expect timeout since every call exits non-zero
	if err == nil {
		t.Fatal("expected error from RunPhase when agent always fails")
	}

	// Verify multiple retry attempts were made (not just one call)
	if n := readCallCount(t, scriptDir); n < 2 {
		t.Fatalf("expected at least 2 mock agent calls (retries), got %d", n)
	}
}

// TestE2EMultiPhaseBlockedDependency verifies that Launch detects when a dependent
// phase cannot run because its dependency is blocked.
func TestE2EMultiPhaseBlockedDependency(t *testing.T) {
	plansDir, _, projectDir := setupE2E(t, "e2e-dep-blocked", []string{"core", "api"}, "feature")

	// Pre-seed "core" as blocked
	coreDir := filepath.Join(plansDir, "e2e-dep-blocked", "phases", "core")
	ps := arc.NewPhaseState("e2e-dep-blocked", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "blocked"
	reason := "stuck"
	ps.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
	writeState(t, coreDir, ps)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := Launch(ctx, LaunchOptions{
		PlanName:   "e2e-dep-blocked",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err == nil {
		t.Fatal("expected Launch to return error when dependency is blocked")
	}
	if !strings.Contains(err.Error(), "no runnable phases") {
		t.Fatalf("expected 'no runnable phases' error, got: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("expected result status blocked, got %q", result.Status)
	}
}

// TestE2EParallelPhasesNoDeps verifies that phases with no dependencies
// run in parallel via Launch(). Two independent "feature" workflow phases
// should both complete without one waiting on the other.
func TestE2EParallelPhasesNoDeps(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-parallel", []string{"alpha", "beta"}, "feature")

	// Override plan.json to remove dependencies (setupE2E creates serial deps)
	planDir := filepath.Join(plansDir, "e2e-parallel")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatal(err)
	}
	meta.Dependencies = map[string][]string{} // no deps — both should be ready
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	// Feature workflow: impl.act → check.adversary(no_bugs_found) → complete
	// Each phase needs 2 calls. With 2 phases in parallel, ordering is nondeterministic.
	// Write scripts: linear states accept any output; adversary needs no_bugs_found verdict.
	// Using no_bugs_found works for both (linear states ignore the verdict section).
	for i := 0; i < 6; i++ {
		writeScript(t, scriptDir, i, "Done.\n\n## Verdict\nno_bugs_found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	result, err := Launch(ctx, LaunchOptions{
		PlanName:   "e2e-parallel",
		PlansDir:   plansDir,
		ArcHome:    t.TempDir(),
		ProjectDir: projectDir,
		Logger:     e2eLogger(),
	})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	elapsed := time.Since(start)
	if result.Status != "complete" {
		t.Fatalf("expected result status complete, got %q", result.Status)
	}

	// Both phases should be complete
	alphaState := readState(t, plansDir, "e2e-parallel", "alpha")
	if alphaState.PhaseStatus != "complete" {
		t.Fatalf("alpha: expected phase_status=complete, got %q", alphaState.PhaseStatus)
	}
	betaState := readState(t, plansDir, "e2e-parallel", "beta")
	if betaState.PhaseStatus != "complete" {
		t.Fatalf("beta: expected phase_status=complete, got %q", betaState.PhaseStatus)
	}

	// Sanity check: both finished in a single orchestrator loop iteration.
	// With sequential execution this would take 2 iterations; with parallel it's 1.
	// We can't measure this directly, but we can check it completed quickly.
	if elapsed > 15*time.Second {
		t.Fatalf("expected parallel phases to complete quickly, took %v", elapsed)
	}
}

// TestE2EStopOnFailure verifies that StopOnFailure returns a LaunchResult with
// Status="failed" (no error) and populates FailedPhase when a phase has a hard failure.
func TestE2EStopOnFailure(t *testing.T) {
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-stopfail", []string{"alpha", "beta"}, "feature")

	// Override plan.json to remove dependencies so both phases are ready.
	planDir := filepath.Join(plansDir, "e2e-stopfail")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatal(err)
	}
	meta.Dependencies = map[string][]string{}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	// Alpha will succeed normally (feature workflow: 4 calls per phase).
	for i := 0; i < 8; i++ {
		writeScript(t, scriptDir, i, "Done.\n\n## Verdict\napproved")
	}

	// Give beta an invalid workflow type so RunPhase fails hard at
	// "loading workflow:" (after successfully reading state, so PhasesReady
	// still considers it ready).
	betaDir := filepath.Join(planDir, "phases", "beta")
	betaState := arc.NewPhaseState("e2e-stopfail", "beta", "feature")
	betaState.WorkflowType = "nonexistent-workflow"
	writeState(t, betaDir, betaState)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := Launch(ctx, LaunchOptions{
		PlanName:      "e2e-stopfail",
		PlansDir:      plansDir,
		ArcHome:       t.TempDir(),
		ProjectDir:    projectDir,
		Logger:        e2eLogger(),
		StopOnFailure: true,
	})
	// StopOnFailure returns nil error — failure goes into the result.
	if err != nil {
		t.Fatalf("expected nil error with StopOnFailure, got: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected result status failed, got %q", result.Status)
	}
	if result.FailedPhase != "beta" {
		t.Fatalf("expected FailedPhase=beta, got %q", result.FailedPhase)
	}
	if result.FailedReason == "" {
		t.Fatal("expected FailedReason to be set")
	}
	if result.PhaseSummary == nil {
		t.Fatal("expected PhaseSummary to be populated")
	}
}


// TestE2EStopOnFailureBlockedPhase verifies that when StopOnFailure is true,
// a phase that becomes blocked during execution causes the orchestrator to
// return a failed result (instead of continuing past it as it does by default).
func TestE2EStopOnFailureBlockedPhase(t *testing.T) {
	// Use "feature" workflow: qa → qa_review → impl → impl_review → complete.
	// A non-zero exit on the first state (qa) causes it to block with ChatMode.
	plansDir, scriptDir, projectDir := setupE2E(t, "e2e-stopblock", []string{"core"}, "feature")

	// Non-zero exit triggers ActionRetry → one retry → blocked.
	t.Setenv("MOCK_EXIT_CODE", "1")
	for i := 0; i < 5; i++ {
		writeScript(t, scriptDir, i, "crash output")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, lErr := Launch(ctx, LaunchOptions{
		PlanName:      "e2e-stopblock",
		PlansDir:      plansDir,
		ArcHome:       t.TempDir(),
		ProjectDir:    projectDir,
		Logger:        e2eLogger(),
		StopOnFailure: true,
		ChatMode:      true,
	})
	// With StopOnFailure, blocked phases should produce a failed result (nil error).
	if lErr != nil {
		t.Fatalf("expected nil error with StopOnFailure, got: %v", lErr)
	}
	if result.Status != "failed" {
		t.Fatalf("expected result status failed, got %q", result.Status)
	}
	if result.FailedPhase != "core" {
		t.Fatalf("expected FailedPhase=core, got %q", result.FailedPhase)
	}
	if !strings.Contains(result.FailedReason, "blocked") {
		t.Fatalf("expected FailedReason to mention 'blocked', got %q", result.FailedReason)
	}
}

// writeScript writes a scripted response file using the proper integer formatting.
func writeScript(t *testing.T, scriptDir string, callIndex int, content string) {
	t.Helper()
	name := fmt.Sprintf("call_%d.txt", callIndex)
	path := filepath.Join(scriptDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
