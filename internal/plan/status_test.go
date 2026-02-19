package plan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestStatusIconMapping(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"pending", "[ ]"},
		{"complete", "[x]"},
		{"implementing", "[>]"},
		{"qa", "[>]"},
		{"qa_review", "[>]"},
		{"disputed", "[!]"},
		{"blocked", "[X]"},
		{"deferred", "[~]"},
		{"split", "[/]"},
		{"unknown_status", "[?]"},
	}
	for _, tc := range tests {
		got := StatusIcon(tc.status)
		if got != tc.want {
			t.Fatalf("StatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestStatusDisplayPending(t *testing.T) {
	dir := setupPlanDir(t, "test-plan", "feature", []string{"phase1", "phase2"}, map[string]string{
		"phase1": "pending",
		"phase2": "pending",
	})

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[ ]") {
		t.Fatalf("output should contain pending icon '[ ]', got:\n%s", output)
	}
}

func TestStatusDisplayMixed(t *testing.T) {
	dir := setupPlanDir(t, "test-plan", "feature", []string{"phase1", "phase2"}, map[string]string{
		"phase1": "complete",
		"phase2": "implementing",
	})

	// Update phase2 state with iteration and test counts
	phase2State := arc.NewPhaseState("test-plan", "phase2", "feature")
	phase2State.PhaseStatus = "implementing"
	phase2State.Iteration.Current = 3
	phase2State.TestsPassing = 5
	phase2State.TestsTotal = 10
	writeStateFile(t, filepath.Join(dir, "test-plan", "phases", "phase2", "state.json"), phase2State)

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[x]") {
		t.Fatalf("output should contain complete icon '[x]', got:\n%s", output)
	}
	if !strings.Contains(output, "[>]") {
		t.Fatalf("output should contain in-progress icon '[>]', got:\n%s", output)
	}
	if !strings.Contains(output, "iter 3") {
		t.Fatalf("output should contain 'iter 3', got:\n%s", output)
	}
	if !strings.Contains(output, "5/10") {
		t.Fatalf("output should contain '5/10', got:\n%s", output)
	}
}

func TestStatusDisplayBlockedBy(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Create plan.json with deps: phase2 depends on phase1
	meta := &arc.PlanMeta{
		Name:         "test-plan",
		Status:       "active",
		Phases:       []string{"phase1", "phase2"},
		PhaseOrder:   map[string]int{"phase1": 1, "phase2": 2},
		Dependencies: map[string][]string{"phase2": {"phase1"}},
		WorkflowType: "feature",
	}
	writePlanJson(t, planDir, meta)

	// phase1 is pending (not complete), phase2 depends on it
	setupPhaseState(t, planDir, "phase1", "pending", "test-plan", "feature")
	setupPhaseState(t, planDir, "phase2", "pending", "test-plan", "feature")

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "BLOCKED BY") {
		t.Fatalf("output should contain 'BLOCKED BY' for phase with unfinished deps, got:\n%s", output)
	}
}

func TestStatusNoPhases(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	meta := &arc.PlanMeta{
		Name:         "test-plan",
		Status:       "active",
		Phases:       []string{},
		PhaseOrder:   map[string]int{},
		Dependencies: map[string][]string{},
		WorkflowType: "feature",
	}
	writePlanJson(t, planDir, meta)

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err != nil {
		t.Fatalf("Status should not crash on empty phases: %v", err)
	}
	// Output should contain plan name but no phase rows
	output := buf.String()
	if !strings.Contains(output, "test-plan") {
		t.Fatalf("output should contain plan name, got:\n%s", output)
	}
}

func TestStatusMissingStateJson(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	meta := &arc.PlanMeta{
		Name:         "test-plan",
		Status:       "active",
		Phases:       []string{"phase1"},
		PhaseOrder:   map[string]int{"phase1": 1},
		Dependencies: map[string][]string{},
		WorkflowType: "feature",
	}
	writePlanJson(t, planDir, meta)

	// Create phase directory without state.json
	if err := os.MkdirAll(filepath.Join(planDir, "phases", "phase1"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	// Should not crash — either returns error or shows error indicator
	if err != nil {
		// An error is acceptable; verify it's a graceful error not a panic
		_ = err
	}
}

func TestStatusCorruptedPlanJson(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Write invalid JSON to plan.json
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err == nil {
		t.Fatal("expected error for corrupted plan.json, got nil")
	}
}

func TestStatusEmptyPlanJson(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Write empty file
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: "test-plan"})
	if err == nil {
		t.Fatal("expected error for empty plan.json, got nil")
	}
}

func TestStatusMultiplePlans(t *testing.T) {
	dir := t.TempDir()

	// Create two plans
	for _, name := range []string{"plan-a", "plan-b"} {
		planDir := filepath.Join(dir, name)
		if err := os.MkdirAll(planDir, 0755); err != nil {
			t.Fatalf("mkdir error: %v", err)
		}
		meta := &arc.PlanMeta{
			Name:         name,
			Status:       "active",
			Phases:       []string{"impl"},
			PhaseOrder:   map[string]int{"impl": 1},
			Dependencies: map[string][]string{},
			WorkflowType: "feature",
		}
		writePlanJson(t, planDir, meta)
		setupPhaseState(t, planDir, "impl", "pending", name, "feature")
	}

	var buf bytes.Buffer
	err := Status(&buf, StatusOptions{PlansDir: dir, PlanName: ""})
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "plan-a") {
		t.Fatalf("output should contain 'plan-a', got:\n%s", output)
	}
	if !strings.Contains(output, "plan-b") {
		t.Fatalf("output should contain 'plan-b', got:\n%s", output)
	}
}

// Helper functions

func setupPlanDir(t *testing.T, planName, workflowType string, phases []string, statuses map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	planDir := filepath.Join(dir, planName)
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	meta := arc.NewPlanMeta(planName, workflowType, phases)
	writePlanJson(t, planDir, meta)

	for _, phase := range phases {
		status := "pending"
		if s, ok := statuses[phase]; ok {
			status = s
		}
		setupPhaseState(t, planDir, phase, status, planName, workflowType)
	}

	return dir
}

func setupPhaseState(t *testing.T, planDir, phase, status, planName, workflowType string) {
	t.Helper()
	phaseDir := filepath.Join(planDir, "phases", phase)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	state := arc.NewPhaseState(planName, phase, workflowType)
	state.PhaseStatus = status
	writeStateFile(t, filepath.Join(phaseDir, "state.json"), state)
}

func writeStateFile(t *testing.T, path string, state *arc.PhaseState) {
	t.Helper()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state error: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write state file error: %v", err)
	}
}

func writePlanJson(t *testing.T, planDir string, meta *arc.PlanMeta) {
	t.Helper()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644); err != nil {
		t.Fatalf("write plan.json error: %v", err)
	}
}
