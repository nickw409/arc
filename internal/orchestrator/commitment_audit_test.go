package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestShouldRunCommitmentAudit_DirectSkips(t *testing.T) {
	meta := &arc.PlanMeta{
		WorkflowType: "direct",
	}
	opts := LaunchOptions{}
	if shouldRunCommitmentAudit(meta, opts) {
		t.Error("shouldRunCommitmentAudit should return false for 'direct' workflow type")
	}
}

func TestShouldRunCommitmentAudit_AllSimpleSkips(t *testing.T) {
	tmpDir := t.TempDir()
	planName := "test-plan"

	// Create plan.md files with complexity: simple
	for _, phaseName := range []string{"phase1", "phase2"} {
		phaseDir := filepath.Join(tmpDir, planName, "phases", phaseName)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}
		spec := &arc.PhaseSpec{Name: phaseName, Spec: "Implement " + phaseName, Complexity: "simple"}
		writeTestPlanMD(t, phaseDir, spec)
	}

	meta := &arc.PlanMeta{
		WorkflowType: "feature",
		Phases:       []string{"phase1", "phase2"},
	}
	opts := LaunchOptions{
		PlansDir: tmpDir,
		PlanName: planName,
	}
	if shouldRunCommitmentAudit(meta, opts) {
		t.Error("shouldRunCommitmentAudit should return false when all phases have complexity 'simple'")
	}
}

func TestShouldRunCommitmentAudit_MediumRuns(t *testing.T) {
	tmpDir := t.TempDir()
	planName := "test-plan"
	phaseName := "phase1"

	phaseDir := filepath.Join(tmpDir, planName, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := &arc.PhaseSpec{Name: phaseName, Spec: "Implement " + phaseName, Complexity: "medium"}
	writeTestPlanMD(t, phaseDir, spec)

	meta := &arc.PlanMeta{
		WorkflowType: "feature",
		Phases:       []string{phaseName},
	}
	opts := LaunchOptions{
		PlansDir: tmpDir,
		PlanName: planName,
	}
	if !shouldRunCommitmentAudit(meta, opts) {
		t.Error("shouldRunCommitmentAudit should return true when a phase has complexity 'medium'")
	}
}

func TestParseGapReport_NoGaps(t *testing.T) {
	output := "NO_GAPS"
	report := parseGapReport(output)
	if len(report.Gaps) != 0 {
		t.Errorf("expected 0 gaps, got %d", len(report.Gaps))
	}
}

func TestParseGapReport_JSONFence(t *testing.T) {
	output := "Here is the analysis:\n\n```json\n{\"gaps\":[{\"phase\":\"daemon\",\"commitment\":\"wire into cli/run.go\",\"file\":\"internal/cli/run.go\",\"pattern\":\"daemon\\\\.Connect\"}]}\n```\n"
	report := parseGapReport(output)
	if len(report.Gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(report.Gaps))
	}
	if report.Gaps[0].Phase != "daemon" {
		t.Errorf("expected phase 'daemon', got %q", report.Gaps[0].Phase)
	}
	if report.Gaps[0].File != "internal/cli/run.go" {
		t.Errorf("expected file 'internal/cli/run.go', got %q", report.Gaps[0].File)
	}
}

func TestParseGapReport_InvalidJSON(t *testing.T) {
	output := "some garbage text with no JSON"
	report := parseGapReport(output)
	if len(report.Gaps) != 0 {
		t.Errorf("expected 0 gaps on parse failure, got %d", len(report.Gaps))
	}
}

func TestParseGapReport_EmptyOutput(t *testing.T) {
	output := ""
	report := parseGapReport(output)
	if len(report.Gaps) != 0 {
		t.Errorf("expected 0 gaps for empty output, got %d", len(report.Gaps))
	}
}
