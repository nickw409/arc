package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestGenerateCompletionReportAllComplete(t *testing.T) {
	planDir := t.TempDir()

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}

	phaseStates := map[string]*arc.PhaseState{
		"phase-a": {
			PhaseStatus:  "complete",
			Iteration:    arc.Iteration{Current: 3},
			TestsPassing: 5,
			TestsTotal:   5,
			LastCommit:   "abc1234",
		},
		"phase-b": {
			PhaseStatus:  "complete",
			Iteration:    arc.Iteration{Current: 2},
			TestsPassing: 3,
			TestsTotal:   3,
		},
	}

	err := generateCompletionReport(planDir, "test-plan", meta, phaseStates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reportPath := filepath.Join(planDir, "COMPLETION_REPORT.md")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report file not found: %v", err)
	}

	report := string(data)

	// Check header
	if !strings.Contains(report, "# Completion Report: test-plan") {
		t.Error("expected report to contain plan name header")
	}

	// Check summary
	if !strings.Contains(report, "2/2 complete") {
		t.Error("expected report to show 2/2 complete")
	}

	// Check phase details
	if !strings.Contains(report, "phase-a") {
		t.Error("expected report to contain phase-a")
	}
	if !strings.Contains(report, "phase-b") {
		t.Error("expected report to contain phase-b")
	}

	// Check commit hash shown for phase-a
	if !strings.Contains(report, "abc1234") {
		t.Error("expected report to contain commit hash for phase-a")
	}

	// Check [x] icon for complete phases
	if !strings.Contains(report, "[x]") {
		t.Error("expected report to contain [x] for complete phases")
	}
}

func TestGenerateCompletionReportWithBlocked(t *testing.T) {
	planDir := t.TempDir()

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}

	phaseStates := map[string]*arc.PhaseState{
		"phase-a": {
			PhaseStatus:  "complete",
			Iteration:    arc.Iteration{Current: 3},
			TestsPassing: 5,
			TestsTotal:   5,
		},
		"phase-b": {
			PhaseStatus:   "blocked",
			Iteration:     arc.Iteration{Current: 8},
			TestsPassing:  1,
			TestsTotal:    3,
			RollbackCount: 2,
		},
	}

	err := generateCompletionReport(planDir, "test-plan", meta, phaseStates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(planDir, "COMPLETION_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}

	report := string(data)

	// Should show blocked count
	if !strings.Contains(report, "Blocked") {
		t.Error("expected report to mention blocked phases")
	}

	// Blocked icon [X]
	if !strings.Contains(report, "[X]") {
		t.Error("expected report to contain [X] for blocked phases")
	}

	// Should show rollback count
	if !strings.Contains(report, "Rollbacks") {
		t.Error("expected report to show rollback count for phase-b")
	}
}

func TestGenerateCompletionReportNilPhaseState(t *testing.T) {
	planDir := t.TempDir()

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a"},
	}

	phaseStates := map[string]*arc.PhaseState{
		"phase-a": nil,
	}

	err := generateCompletionReport(planDir, "test-plan", meta, phaseStates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(planDir, "COMPLETION_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}

	report := string(data)
	if !strings.Contains(report, "No state found") {
		t.Error("expected report to contain 'No state found' for nil phase state")
	}
}

func TestGenerateCompletionReportWithUsage(t *testing.T) {
	planDir := t.TempDir()

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}

	phaseStates := map[string]*arc.PhaseState{
		"phase-a": {
			PhaseStatus:  "complete",
			Iteration:    arc.Iteration{Current: 3},
			TestsPassing: 5,
			TestsTotal:   5,
			Usage: arc.Usage{
				InputTokens:  1000,
				OutputTokens: 500,
				CostUSD:      0.05,
			},
		},
		"phase-b": {
			PhaseStatus:  "complete",
			Iteration:    arc.Iteration{Current: 2},
			TestsPassing: 3,
			TestsTotal:   3,
			Usage: arc.Usage{
				InputTokens:  2000,
				OutputTokens: 800,
				CostUSD:      0.10,
			},
		},
	}

	err := generateCompletionReport(planDir, "test-plan", meta, phaseStates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(planDir, "COMPLETION_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}

	report := string(data)

	// Check per-phase cost
	if !strings.Contains(report, "$0.05") {
		t.Error("expected report to contain per-phase cost $0.05")
	}
	if !strings.Contains(report, "$0.10") {
		t.Error("expected report to contain per-phase cost $0.10")
	}

	// Check cost summary section
	if !strings.Contains(report, "## Cost Summary") {
		t.Error("expected report to contain cost summary section")
	}
	if !strings.Contains(report, "$0.15") {
		t.Error("expected report to contain total cost $0.15")
	}
	if !strings.Contains(report, "3000") {
		t.Error("expected report to contain total input tokens 3000")
	}
}

func TestGenerateCompletionReportSplitAndDeferred(t *testing.T) {
	planDir := t.TempDir()

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}

	phaseStates := map[string]*arc.PhaseState{
		"phase-a": {
			PhaseStatus: "split",
			Iteration:   arc.Iteration{Current: 1},
		},
		"phase-b": {
			PhaseStatus: "deferred",
			Iteration:   arc.Iteration{Current: 0},
		},
	}

	err := generateCompletionReport(planDir, "test-plan", meta, phaseStates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(planDir, "COMPLETION_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}

	report := string(data)

	// split and deferred count as "complete" in the summary
	if !strings.Contains(report, "2/2 complete") {
		t.Error("expected split/deferred to count as complete in summary")
	}

	// Check icons
	if !strings.Contains(report, "[/]") {
		t.Error("expected report to contain [/] for split phases")
	}
	if !strings.Contains(report, "[-]") {
		t.Error("expected report to contain [-] for deferred phases")
	}
}
