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

	// Override both command name vars so all agent spawns use the mock.
	pipeline.SetAgentCommandNameForTest(bin)
	judgeCommandName = bin

	os.Exit(m.Run())
}

// setupE2E creates a plan directory with phases and returns (plansDir, scriptDir).
func setupE2E(t *testing.T, planName string, phases []string, workflowType string) (string, string) {
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

	return plansDir, scriptDir
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
// qa → qa_review(approved) → impl → impl_review(approved) → complete
func TestE2EHappyPath(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-happy", []string{"core"}, "feature")

	// Call 0: qa (linear, no verdict needed)
	writeScript(t, scriptDir, 0, "QA tests written successfully.")
	// Call 1: qa_review → approved
	writeScript(t, scriptDir, 1, "Review complete.\n\n## Verdict\napproved")
	// Call 2: impl (linear, no verdict needed)
	writeScript(t, scriptDir, 2, "Implementation complete.")
	// Call 3: impl_review → approved
	writeScript(t, scriptDir, 3, "Implementation looks good.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-happy",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
	// 4 agent calls = 4 iterations
	if ps.Iteration.Current != 4 {
		t.Fatalf("expected iteration.current=4, got %d", ps.Iteration.Current)
	}
	if ps.GlobalIterations != 4 {
		t.Fatalf("expected global_iterations=4, got %d", ps.GlobalIterations)
	}
	if ps.LastVerdict != "approved" {
		t.Fatalf("expected last_verdict=approved, got %q", ps.LastVerdict)
	}
	// Exactly 2 verdicts: qa_review→approved, impl_review→approved
	if len(ps.VerdictsHistory) != 2 {
		t.Fatalf("expected 2 verdicts in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "approved" || ps.VerdictsHistory[0].State != "qa_review" {
		t.Fatalf("unexpected first verdict: %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "approved" || ps.VerdictsHistory[1].State != "impl_review" {
		t.Fatalf("unexpected second verdict: %+v", ps.VerdictsHistory[1])
	}
	// Verify exactly 4 mock calls were made
	if n := readCallCount(t, scriptDir); n != 4 {
		t.Fatalf("expected 4 mock agent calls, got %d", n)
	}
}

// TestE2EQAGapsLoop exercises the QA gaps loop:
// qa → qa_review(gaps_found) → qa → qa_review(approved) → impl → impl_review(approved) → complete
func TestE2EQAGapsLoop(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-gaps", []string{"core"}, "feature")

	// Call 0: qa
	writeScript(t, scriptDir, 0, "QA tests written.")
	// Call 1: qa_review → gaps_found
	writeScript(t, scriptDir, 1, "Gaps found.\n\n## Verdict\ngaps_found")
	// Call 2: qa (re-run)
	writeScript(t, scriptDir, 2, "Additional tests written.")
	// Call 3: qa_review → approved
	writeScript(t, scriptDir, 3, "Tests now adequate.\n\n## Verdict\napproved")
	// Call 4: impl
	writeScript(t, scriptDir, 4, "Implementation complete.")
	// Call 5: impl_review → approved
	writeScript(t, scriptDir, 5, "Looks good.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-gaps",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
	if ps.Iteration.Current != 6 {
		t.Fatalf("expected iteration.current=6, got %d", ps.Iteration.Current)
	}
	if ps.GlobalIterations != 6 {
		t.Fatalf("expected global_iterations=6, got %d", ps.GlobalIterations)
	}
	// 3 verdicts: gaps_found, approved (qa_review), approved (impl_review)
	if len(ps.VerdictsHistory) != 3 {
		t.Fatalf("expected 3 verdicts in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "gaps_found" || ps.VerdictsHistory[0].State != "qa_review" {
		t.Fatalf("expected first verdict gaps_found@qa_review, got %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "approved" || ps.VerdictsHistory[1].State != "qa_review" {
		t.Fatalf("expected second verdict approved@qa_review, got %+v", ps.VerdictsHistory[1])
	}
	if ps.VerdictsHistory[2].Verdict != "approved" || ps.VerdictsHistory[2].State != "impl_review" {
		t.Fatalf("expected third verdict approved@impl_review, got %+v", ps.VerdictsHistory[2])
	}
	if n := readCallCount(t, scriptDir); n != 6 {
		t.Fatalf("expected 6 mock agent calls, got %d", n)
	}
}

// TestE2EImplConcerns exercises the impl concerns loop:
// qa → qa_review(approved) → impl → impl_review(concerns) → impl → impl_review(approved) → complete
func TestE2EImplConcerns(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-concerns", []string{"core"}, "feature")

	// Call 0: qa
	writeScript(t, scriptDir, 0, "QA tests written.")
	// Call 1: qa_review → approved
	writeScript(t, scriptDir, 1, "Tests look good.\n\n## Verdict\napproved")
	// Call 2: impl
	writeScript(t, scriptDir, 2, "First implementation attempt.")
	// Call 3: impl_review → concerns
	writeScript(t, scriptDir, 3, "Has issues.\n\n## Verdict\nconcerns")
	// Call 4: impl (re-run)
	writeScript(t, scriptDir, 4, "Fixed implementation.")
	// Call 5: impl_review → approved
	writeScript(t, scriptDir, 5, "Now looks good.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-concerns",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-concerns", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete, got %q", ps.CurrentState)
	}
	if ps.PhaseStatus != "complete" {
		t.Fatalf("expected phase_status=complete, got %q", ps.PhaseStatus)
	}
	if ps.Iteration.Current != 6 {
		t.Fatalf("expected iteration.current=6, got %d", ps.Iteration.Current)
	}
	if ps.GlobalIterations != 6 {
		t.Fatalf("expected global_iterations=6, got %d", ps.GlobalIterations)
	}
	// 3 verdicts: approved (qa_review), concerns (impl_review), approved (impl_review)
	if len(ps.VerdictsHistory) != 3 {
		t.Fatalf("expected 3 verdicts in history, got %d: %+v", len(ps.VerdictsHistory), ps.VerdictsHistory)
	}
	if ps.VerdictsHistory[0].Verdict != "approved" || ps.VerdictsHistory[0].State != "qa_review" {
		t.Fatalf("expected first verdict approved@qa_review, got %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "concerns" || ps.VerdictsHistory[1].State != "impl_review" {
		t.Fatalf("expected second verdict concerns@impl_review, got %+v", ps.VerdictsHistory[1])
	}
	if ps.VerdictsHistory[2].Verdict != "approved" || ps.VerdictsHistory[2].State != "impl_review" {
		t.Fatalf("expected third verdict approved@impl_review, got %+v", ps.VerdictsHistory[2])
	}
	if n := readCallCount(t, scriptDir); n != 6 {
		t.Fatalf("expected 6 mock agent calls, got %d", n)
	}
}

// TestE2EDisputeApproved exercises the dispute-approved flow:
// JudgeDispute → APPROVE_DISPUTE → fix iteration → impl continues → impl_review(approved) → complete
func TestE2EDisputeApproved(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-dispute-ok", []string{"core"}, "feature")

	// Pre-seed state to disputed status in impl state
	phaseDir := filepath.Join(plansDir, "e2e-dispute-ok", "phases", "core")
	ps := arc.NewPhaseState("e2e-dispute-ok", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "disputed"
	ps.Disputes = []arc.Dispute{
		{TestName: "TestFoo", Reason: "test is wrong"},
	}
	writeState(t, phaseDir, ps)

	// Call 0: JudgeDispute → approve
	writeScript(t, scriptDir, 0, "APPROVE_DISPUTE: test wrong")
	// Call 1: fix iteration (linear impl state, agent does fix work)
	writeScript(t, scriptDir, 1, "Fixed the test.")
	// Call 2: impl iteration (back to impl after dispute clear, linear → impl_review)
	writeScript(t, scriptDir, 2, "Implementation done.")
	// Call 3: impl_review → approved
	writeScript(t, scriptDir, 3, "All good.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-dispute-ok",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
// JudgeDispute → REJECT_DISPUTE → impl continues → impl_review(approved) → complete
func TestE2EDisputeRejected(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-dispute-no", []string{"core"}, "feature")

	// Pre-seed state to disputed status in impl state
	phaseDir := filepath.Join(plansDir, "e2e-dispute-no", "phases", "core")
	ps := arc.NewPhaseState("e2e-dispute-no", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "disputed"
	ps.Disputes = []arc.Dispute{
		{TestName: "TestBar", Reason: "impl is wrong"},
	}
	writeState(t, phaseDir, ps)

	// Call 0: JudgeDispute → reject
	writeScript(t, scriptDir, 0, "REJECT_DISPUTE: impl wrong")
	// Call 1: impl iteration continues (state stays impl, linear → impl_review)
	writeScript(t, scriptDir, 1, "Implementation fixed.")
	// Call 2: impl_review → approved
	writeScript(t, scriptDir, 2, "Looks good.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-dispute-no",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
	plansDir, scriptDir := setupE2E(t, "e2e-multi", []string{"core", "api"}, "feature")

	// Phase "core": calls 0-3 (happy path)
	writeScript(t, scriptDir, 0, "QA tests for core.")
	writeScript(t, scriptDir, 1, "## Verdict\napproved")
	writeScript(t, scriptDir, 2, "Core implementation.")
	writeScript(t, scriptDir, 3, "## Verdict\napproved")

	// Phase "api": calls 4-7 (happy path)
	writeScript(t, scriptDir, 4, "QA tests for api.")
	writeScript(t, scriptDir, 5, "## Verdict\napproved")
	writeScript(t, scriptDir, 6, "API implementation.")
	writeScript(t, scriptDir, 7, "## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := Launch(ctx, LaunchOptions{
		PlanName: "e2e-multi",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Logger:   e2eLogger(),
	})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
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

	// Verify all 8 mock calls were consumed
	if n := readCallCount(t, scriptDir); n != 8 {
		t.Fatalf("expected 8 mock agent calls, got %d", n)
	}
}

// TestE2EEscalationBlocked verifies that a phase becomes blocked when stuck
// iterations and rollback count are exhausted via handleEscalation.
func TestE2EEscalationBlocked(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-blocked", []string{"core"}, "feature")

	// Pre-seed state with high stuck_iterations and rollback_count
	phaseDir := filepath.Join(plansDir, "e2e-blocked", "phases", "core")
	ps := arc.NewPhaseState("e2e-blocked", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "implementing"
	ps.StuckIterations = 6
	ps.RollbackCount = 2
	writeState(t, phaseDir, ps)

	// The feature workflow has no escalation rules in the YAML, so
	// CheckEscalation in the pipeline returns nil. The escalation path is
	// driven by handleEscalation in phase.go when RunPhase receives
	// ActionEscalate. We test handleEscalation directly because the pipeline
	// won't produce ActionEscalate without escalation rules in the workflow.
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))

	writeScript(t, scriptDir, 0, "stuck output")

	err := handleEscalation(context.Background(), RunPhaseOptions{
		PlanName:  "e2e-blocked",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	}, sf, ps)
	if err != nil {
		t.Fatalf("handleEscalation failed: %v", err)
	}

	ps = readState(t, plansDir, "e2e-blocked", "core")
	if ps.PhaseStatus != "blocked" {
		t.Fatalf("expected phase_status=blocked, got %q", ps.PhaseStatus)
	}
	if !ps.Blocked.IsBlocked {
		t.Fatal("expected blocked.is_blocked=true")
	}
	if ps.Blocked.Reason == nil || *ps.Blocked.Reason != "max rollbacks exhausted" {
		t.Fatalf("expected blocked reason 'max rollbacks exhausted', got %v", ps.Blocked.Reason)
	}
	if ps.CurrentState != "impl" {
		t.Fatalf("expected current_state to remain impl, got %q", ps.CurrentState)
	}
}

// TestE2EEscalationRollback verifies that handleEscalation performs a rollback
// when stuck_iterations >= 6 but rollback_count < 2.
func TestE2EEscalationRollback(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-rollback", []string{"core"}, "feature")

	phaseDir := filepath.Join(plansDir, "e2e-rollback", "phases", "core")
	ps := arc.NewPhaseState("e2e-rollback", "core", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "implementing"
	ps.StuckIterations = 6
	ps.RollbackCount = 0
	ps.Iteration.Current = 10
	ps.TestsPassing = 3
	ps.TestsTotal = 5
	writeState(t, phaseDir, ps)

	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	writeScript(t, scriptDir, 0, "unused")

	err := handleEscalation(context.Background(), RunPhaseOptions{
		PlanName:  "e2e-rollback",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	}, sf, ps)
	if err != nil {
		t.Fatalf("handleEscalation failed: %v", err)
	}

	ps = readState(t, plansDir, "e2e-rollback", "core")
	// Should have rolled back, not blocked
	if ps.PhaseStatus == "blocked" {
		t.Fatal("expected rollback, not blocked")
	}
	if ps.RollbackCount != 1 {
		t.Fatalf("expected rollback_count=1, got %d", ps.RollbackCount)
	}
	if ps.Iteration.Current != 0 {
		t.Fatalf("expected iteration.current reset to 0, got %d", ps.Iteration.Current)
	}
	if ps.StuckIterations != 0 {
		t.Fatalf("expected stuck_iterations reset to 0, got %d", ps.StuckIterations)
	}
	if ps.TestsPassing != 0 || ps.TestsTotal != 0 {
		t.Fatalf("expected test counts reset, got %d/%d", ps.TestsPassing, ps.TestsTotal)
	}
}

// TestE2EBlockedPhaseReturnsError verifies that RunPhase returns an error
// immediately when a phase has phase_status=blocked.
func TestE2EBlockedPhaseReturnsError(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-preblocked", []string{"core"}, "feature")

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
	plansDir, scriptDir := setupE2E(t, "e2e-cancel", []string{"core"}, "feature")

	// Call 0: qa (this will succeed)
	writeScript(t, scriptDir, 0, "QA tests written.")
	// No further scripts — the phase will try to proceed to qa_review and
	// call the mock agent again. With MOCK_SCRIPT_DIR, call_1.txt is missing
	// so it falls through to MOCK_OUTPUT (empty), yielding empty output which
	// triggers ActionRetry in the loop. We cancel the context before that.

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-cancel",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
	plansDir, scriptDir := setupE2E(t, "e2e-badverdict", []string{"core"}, "feature")

	// Call 0: qa (linear, succeeds)
	writeScript(t, scriptDir, 0, "QA tests written.")
	// Call 1: qa_review — missing ## Verdict header → triggers retry
	writeScript(t, scriptDir, 1, "This review has no verdict section at all.")
	// Call 2: qa_review retry — now with valid verdict
	writeScript(t, scriptDir, 2, "Better review.\n\n## Verdict\napproved")
	// Call 3: impl
	writeScript(t, scriptDir, 3, "Implementation done.")
	// Call 4: impl_review → approved
	writeScript(t, scriptDir, 4, "Looks great.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-badverdict",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-badverdict", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete after retry, got %q", ps.CurrentState)
	}
	// 5 calls total: qa, bad qa_review, good qa_review, impl, impl_review
	if n := readCallCount(t, scriptDir); n != 5 {
		t.Fatalf("expected 5 mock agent calls (including retry), got %d", n)
	}
}

// TestE2EEmptyOutputRetries verifies that an agent returning empty output
// triggers a retry rather than crashing.
func TestE2EEmptyOutputRetries(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-empty", []string{"core"}, "feature")

	// Call 0: qa — empty output → retry
	writeScript(t, scriptDir, 0, "")
	// Call 1: qa — retry with real output
	writeScript(t, scriptDir, 1, "QA tests written.")
	// Call 2: qa_review → approved
	writeScript(t, scriptDir, 2, "Good.\n\n## Verdict\napproved")
	// Call 3: impl
	writeScript(t, scriptDir, 3, "Implementation done.")
	// Call 4: impl_review → approved
	writeScript(t, scriptDir, 4, "LGTM.\n\n## Verdict\napproved")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-empty",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
	})
	if err != nil {
		t.Fatalf("RunPhase failed: %v", err)
	}

	ps := readState(t, plansDir, "e2e-empty", "core")
	if ps.CurrentState != "complete" {
		t.Fatalf("expected complete after retry, got %q", ps.CurrentState)
	}
	// 5 calls: empty qa, real qa, qa_review, impl, impl_review
	if n := readCallCount(t, scriptDir); n != 5 {
		t.Fatalf("expected 5 mock agent calls (including retry), got %d", n)
	}
}

// TestE2ENonZeroExitRetries verifies that an agent exiting with non-zero
// status triggers a retry.
func TestE2ENonZeroExitRetries(t *testing.T) {
	plansDir, scriptDir := setupE2E(t, "e2e-exit", []string{"core"}, "feature")
	t.Setenv("MOCK_EXIT_CODE", "1")

	// Call 0: qa — will exit with code 1 → retry
	writeScript(t, scriptDir, 0, "fail")

	// After call 0, clear exit code so subsequent calls succeed
	// We can't change env mid-run, so instead we use a short timeout
	// and verify the retry happened.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := RunPhase(ctx, RunPhaseOptions{
		PlanName:  "e2e-exit",
		PhaseName: "core",
		PlansDir:  plansDir,
		ArcHome:   t.TempDir(),
		Logger:    e2eLogger(),
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
	plansDir, _ := setupE2E(t, "e2e-dep-blocked", []string{"core", "api"}, "feature")

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

	err := Launch(ctx, LaunchOptions{
		PlanName: "e2e-dep-blocked",
		PlansDir: plansDir,
		ArcHome:  t.TempDir(),
		Logger:   e2eLogger(),
	})
	if err == nil {
		t.Fatal("expected Launch to return error when dependency is blocked")
	}
	if !strings.Contains(err.Error(), "no runnable phases") {
		t.Fatalf("expected 'no runnable phases' error, got: %v", err)
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
