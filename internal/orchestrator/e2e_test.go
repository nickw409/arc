package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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

// writeScriptFile writes a scripted response for the mock agent at the given call index.
func writeScriptFile(t *testing.T, scriptDir string, callIndex int, content string) {
	t.Helper()
	path := filepath.Join(scriptDir, "call_"+itoa(callIndex)+".txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// setupE2E creates a plan directory with phases and returns (plansDir, scriptDir, cleanup).
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

func e2eLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// Format helpers for itoa — use stdlib instead
func init() {
	// Override the hacky itoa above with stdlib
}

// Override itoa with proper version
func formatInt(n int) string {
	s := ""
	if n < 10 {
		return string(rune('0' + n))
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
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
	if len(ps.VerdictsHistory) != 2 {
		t.Fatalf("expected 2 verdicts in history, got %d", len(ps.VerdictsHistory))
	}
	if ps.VerdictsHistory[0].Verdict != "approved" || ps.VerdictsHistory[0].State != "qa_review" {
		t.Fatalf("unexpected first verdict: %+v", ps.VerdictsHistory[0])
	}
	if ps.VerdictsHistory[1].Verdict != "approved" || ps.VerdictsHistory[1].State != "impl_review" {
		t.Fatalf("unexpected second verdict: %+v", ps.VerdictsHistory[1])
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

	// Should have gaps_found in history
	foundGaps := false
	for _, v := range ps.VerdictsHistory {
		if v.Verdict == "gaps_found" {
			foundGaps = true
			break
		}
	}
	if !foundGaps {
		t.Fatal("expected gaps_found in verdicts history")
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

	foundConcerns := false
	for _, v := range ps.VerdictsHistory {
		if v.Verdict == "concerns" {
			foundConcerns = true
			break
		}
	}
	if !foundConcerns {
		t.Fatal("expected concerns in verdicts history")
	}
}

// TestE2EDisputeApproved exercises the dispute-approved flow:
// JudgeDispute → APPROVE_DISPUTE → fix iteration → impl_review(approved) → complete
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
	if len(ps.Disputes) != 0 {
		t.Fatalf("expected disputes cleared, got %d", len(ps.Disputes))
	}
	if len(ps.LastClearedDisputes) == 0 {
		t.Fatal("expected last_cleared_disputes to be populated")
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
	if len(ps.Disputes) != 0 {
		t.Fatalf("expected disputes cleared, got %d", len(ps.Disputes))
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
	apiState := readState(t, plansDir, "e2e-multi", "api")
	if apiState.CurrentState != "complete" {
		t.Fatalf("api: expected complete, got %q", apiState.CurrentState)
	}

	// Verify completion report generated
	reportPath := filepath.Join(plansDir, "e2e-multi", "COMPLETION_REPORT.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatal("expected COMPLETION_REPORT.md to be generated")
	}
}

// TestE2EEscalationBlocked verifies that a phase becomes blocked when stuck
// iterations and rollback count are exhausted.
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

	// The mock agent will be called for the impl iteration, which will trigger
	// escalation check. The iteration itself needs to produce an escalation result.
	// However, escalation is checked in RunIteration via CheckEscalation, and then
	// handleEscalation is called in RunPhase. We need the agent to return something
	// that triggers the escalation path.
	//
	// Looking at iterate.go: CheckEscalation runs before the agent spawn. If it
	// returns non-nil, RunIteration returns ActionEscalate. Then RunPhase calls
	// handleEscalation, which sees stuck>=6 and rollback>=2, sets blocked.
	//
	// But we need an escalation rule in the workflow for CheckEscalation to trigger.
	// The feature workflow doesn't have escalation rules, so CheckEscalation returns nil.
	// Instead, the escalation is handled by the "stuck" logic in phase.go lines 96-101
	// which generates instructions, and the actual escalation happens via ActionEscalate
	// from RunIteration when pipeline.CheckEscalation fires.
	//
	// Since the feature workflow has no escalation rules, we test handleEscalation directly.
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))

	// Provide a mock response in case the agent is called
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
}

// writeScript writes a scripted response file using the proper integer formatting.
func writeScript(t *testing.T, scriptDir string, callIndex int, content string) {
	t.Helper()
	name := "call_" + intToStr(callIndex) + ".txt"
	path := filepath.Join(scriptDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
