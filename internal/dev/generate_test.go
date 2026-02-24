package dev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/workflow"
)

// --- GenerateSimplePlanMD tests ---

func TestGenerateSimplePlanMD_ContainsRequiredSections(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:   "Fix off-by-one error in pagination",
		Approach:      "Adjust boundary check in paginate() function",
		RelevantFiles: []FileRef{{Path: "internal/api/paginate.go", Description: "pagination logic"}},
		Requirements:  []string{"Fix the boundary check", "Add test for edge case"},
		Complexity:    ComplexitySimple,
		WorkflowType:  "direct",
	}

	result := GenerateSimplePlanMD(discovery)

	checks := []struct {
		label    string
		contains string
	}{
		{"objective section", "## Objective"},
		{"task summary", "Fix off-by-one"},
		{"relevant file", "paginate.go"},
		{"requirements section", "## Requirements"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.contains) {
			t.Errorf("GenerateSimplePlanMD missing %s: want containing %q, got:\n%s", c.label, c.contains, result)
		}
	}
}

func TestGenerateSimplePlanMD_EmptyApproach(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:   "Fix the typo in README",
		Approach:      "",
		RelevantFiles: []FileRef{{Path: "README.md", Description: "readme"}},
		Requirements:  []string{"Fix typo"},
		Complexity:    ComplexitySimple,
		WorkflowType:  "direct",
	}

	result := GenerateSimplePlanMD(discovery)

	// Should still contain other sections even without approach
	if !strings.Contains(result, "Fix the typo") {
		t.Errorf("expected task summary in output, got:\n%s", result)
	}
	if !strings.Contains(result, "README.md") {
		t.Errorf("expected relevant file in output, got:\n%s", result)
	}
}

func TestGenerateSimplePlanMD_EmptyTaskSummary(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:   "",
		Approach:      "Some approach",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		Complexity:    ComplexitySimple,
		WorkflowType:  "direct",
	}

	// Should not panic — graceful degradation
	result := GenerateSimplePlanMD(discovery)

	// Should still include the objective section heading, even if empty
	if !strings.Contains(result, "## Objective") {
		t.Errorf("expected '## Objective' heading even with empty task summary, got:\n%s", result)
	}
}

// --- GeneratePhasePlanMD tests ---

func TestGeneratePhasePlanMD_ContainsPhaseInfo(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:   "Add auth",
		RelevantFiles: []FileRef{{Path: "auth.go", Description: "auth"}},
		Complexity:    ComplexityMedium,
		WorkflowType:  "bugfix",
	}
	phase := PhaseSpec{Name: "auth-types", Description: "Define auth types"}

	result := GeneratePhasePlanMD(discovery, phase)

	checks := []struct {
		label    string
		contains string
	}{
		{"phase name", "auth-types"},
		{"phase description", "Define auth types"},
		{"relevant file", "auth.go"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.contains) {
			t.Errorf("GeneratePhasePlanMD missing %s: want containing %q, got:\n%s", c.label, c.contains, result)
		}
	}
}

func TestGeneratePhasePlanMD_EmptyPhaseDescription(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:   "Implement feature",
		RelevantFiles: []FileRef{},
		Complexity:    ComplexityMedium,
		WorkflowType:  "feature",
	}
	phase := PhaseSpec{Name: "impl", Description: ""}

	result := GeneratePhasePlanMD(discovery, phase)

	// Phase name should appear even if description is empty
	if !strings.Contains(result, "impl") {
		t.Errorf("expected phase name 'impl' in output, got:\n%s", result)
	}
}

// --- BuildCustomWorkflow tests ---

func TestBuildCustomWorkflow_ThreePhases(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "a", Description: "phase a"},
		{Name: "b", Description: "phase b"},
		{Name: "c", Description: "phase c"},
	}

	yamlBytes, err := BuildCustomWorkflow("custom", phases)
	if err != nil {
		t.Fatalf("BuildCustomWorkflow error: %v", err)
	}

	// Must parse without syntax errors
	w, err := workflow.LoadBytes(yamlBytes)
	if err != nil {
		t.Fatalf("generated YAML failed to load: %v\n\nYAML:\n%s", err, string(yamlBytes))
	}

	// Entry state is "a"
	if w.EntryState != "a" {
		t.Errorf("EntryState = %q, want %q", w.EntryState, "a")
	}

	// Terminal states are [complete, blocked]
	terminalSet := make(map[string]bool)
	for _, ts := range w.TerminalStates {
		terminalSet[ts] = true
	}
	if !terminalSet["complete"] || !terminalSet["blocked"] {
		t.Errorf("TerminalStates = %v, want [complete, blocked]", w.TerminalStates)
	}

	// 8 total states: a, a_review, b, b_review, c, c_review, complete, blocked
	if len(w.States) != 8 {
		t.Errorf("States count = %d, want 8", len(w.States))
	}

	// Build state lookup
	stateMap := make(map[string]arc.StateConfig)
	for _, s := range w.States {
		stateMap[s.Name] = s
	}

	expectedStates := []string{"a", "a_review", "b", "b_review", "c", "c_review", "complete", "blocked"}
	for _, name := range expectedStates {
		if _, ok := stateMap[name]; !ok {
			t.Errorf("missing expected state %q", name)
		}
	}

	// a_review approved → b
	aReview := stateMap["a_review"]
	if aReview.Transition.Branches[arc.Verdict("approved")] != "b" {
		t.Errorf("a_review approved → %q, want %q", aReview.Transition.Branches[arc.Verdict("approved")], "b")
	}

	// c_review approved → complete
	cReview := stateMap["c_review"]
	if cReview.Transition.Branches[arc.Verdict("approved")] != "complete" {
		t.Errorf("c_review approved → %q, want %q", cReview.Transition.Branches[arc.Verdict("approved")], "complete")
	}

	// b_review concerns → b
	bReview := stateMap["b_review"]
	if bReview.Transition.Branches[arc.Verdict("concerns")] != "b" {
		t.Errorf("b_review concerns → %q, want %q", bReview.Transition.Branches[arc.Verdict("concerns")], "b")
	}
}

func TestBuildCustomWorkflow_SinglePhase(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "impl", Description: "implement"},
	}

	yamlBytes, err := BuildCustomWorkflow("custom", phases)
	if err != nil {
		t.Fatalf("BuildCustomWorkflow error: %v", err)
	}

	w, err := workflow.LoadBytes(yamlBytes)
	if err != nil {
		t.Fatalf("generated YAML failed to load: %v\n\nYAML:\n%s", err, string(yamlBytes))
	}

	// States: impl, impl_review, complete, blocked (4 total)
	if len(w.States) != 4 {
		t.Errorf("States count = %d, want 4", len(w.States))
	}

	stateMap := make(map[string]arc.StateConfig)
	for _, s := range w.States {
		stateMap[s.Name] = s
	}

	// impl_review approved → complete
	implReview := stateMap["impl_review"]
	if implReview.Transition.Branches[arc.Verdict("approved")] != "complete" {
		t.Errorf("impl_review approved → %q, want %q", implReview.Transition.Branches[arc.Verdict("approved")], "complete")
	}
}

func TestBuildCustomWorkflow_EmptyPhases(t *testing.T) {
	_, err := BuildCustomWorkflow("custom", []PhaseSpec{})
	if err == nil {
		t.Fatal("expected error for empty phases, got nil")
	}
}

func TestBuildCustomWorkflow_PhaseNameWithHyphens(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "auth-types", Description: "phase"},
	}

	yamlBytes, err := BuildCustomWorkflow("custom", phases)
	if err != nil {
		t.Fatalf("BuildCustomWorkflow error: %v", err)
	}

	w, err := workflow.LoadBytes(yamlBytes)
	if err != nil {
		t.Fatalf("generated YAML failed to load: %v\n\nYAML:\n%s", err, string(yamlBytes))
	}

	stateMap := make(map[string]arc.StateConfig)
	for _, s := range w.States {
		stateMap[s.Name] = s
	}

	if _, ok := stateMap["auth-types"]; !ok {
		t.Error("missing state 'auth-types'")
	}
	if _, ok := stateMap["auth-types_review"]; !ok {
		t.Error("missing state 'auth-types_review'")
	}
}

func TestBuildCustomWorkflow_DuplicatePhaseNames(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "impl", Description: "a"},
		{Name: "impl", Description: "b"},
	}

	_, err := BuildCustomWorkflow("custom", phases)
	if err == nil {
		t.Fatal("expected error for duplicate phase names, got nil")
	}
}

func TestBuildCustomWorkflow_PhaseNameConflictsWithTerminalState(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "complete", Description: "my phase"},
	}

	_, err := BuildCustomWorkflow("custom", phases)
	if err == nil {
		t.Fatal("expected error for phase name conflicting with terminal state, got nil")
	}
}

func TestBuildCustomWorkflow_PhaseNameWithSpecialChars(t *testing.T) {
	phases := []PhaseSpec{
		{Name: "my phase", Description: "has space"},
	}

	_, err := BuildCustomWorkflow("custom", phases)
	if err == nil {
		t.Fatal("expected error for phase name with spaces, got nil")
	}
}

// --- BuildCustomWorkflow edge case: WorkflowType "custom" ---

func TestBuildCustomWorkflow_WorkflowTypeCustom(t *testing.T) {
	// When discovery has WorkflowType "custom", GeneratePlan should use
	// BuildCustomWorkflow. This test verifies BuildCustomWorkflow produces
	// valid YAML with WorkflowType=custom that can be used by the loader.
	phases := []PhaseSpec{
		{Name: "design", Description: "design the system"},
		{Name: "implement", Description: "implement the design"},
	}

	yamlBytes, err := BuildCustomWorkflow("custom", phases)
	if err != nil {
		t.Fatalf("BuildCustomWorkflow error: %v", err)
	}

	w, err := workflow.LoadBytes(yamlBytes)
	if err != nil {
		t.Fatalf("custom workflow YAML failed to load: %v\n\nYAML:\n%s", err, string(yamlBytes))
	}

	if w.EntryState != "design" {
		t.Errorf("EntryState = %q, want %q", w.EntryState, "design")
	}
}

// --- GeneratePlan tests ---

func TestGeneratePlan_SimpleTask(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := GeneratePlan(GenerateOptions{
		PlanName: "fix-typo",
		PlansDir: tmpDir,
		Discovery: &DiscoveryResult{
			Complexity:      ComplexitySimple,
			TaskSummary:     "Fix typo",
			WorkflowType:    "direct",
			SuggestedPhases: []PhaseSpec{{Name: "execute", Description: "fix it"}},
			RelevantFiles:   []FileRef{},
			Requirements:    []string{},
			Approach:        "Fix the typo in README.md",
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan error: %v", err)
	}
	if meta == nil {
		t.Fatal("GeneratePlan returned nil meta")
	}

	// Check WorkflowType
	if meta.WorkflowType != "direct" {
		t.Errorf("WorkflowType = %q, want %q", meta.WorkflowType, "direct")
	}

	// Check Phases
	if len(meta.Phases) != 1 || meta.Phases[0] != "execute" {
		t.Errorf("Phases = %v, want [execute]", meta.Phases)
	}

	// Check plan.md contains task summary
	planMD, err := os.ReadFile(filepath.Join(tmpDir, "fix-typo", "phases", "execute", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md error: %v", err)
	}
	if !strings.Contains(string(planMD), "Fix typo") {
		t.Errorf("plan.md should contain 'Fix typo', got:\n%s", string(planMD))
	}

	// Check state.json has correct WorkflowType
	stateData, err := os.ReadFile(filepath.Join(tmpDir, "fix-typo", "phases", "execute", "state.json"))
	if err != nil {
		t.Fatalf("ReadFile state.json error: %v", err)
	}
	var state arc.PhaseState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("Unmarshal state.json error: %v", err)
	}
	if state.WorkflowType != "direct" {
		t.Errorf("state.WorkflowType = %q, want %q", state.WorkflowType, "direct")
	}
}

func TestGeneratePlan_MediumTask(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := GeneratePlan(GenerateOptions{
		PlanName: "fix-bug",
		PlansDir: tmpDir,
		Discovery: &DiscoveryResult{
			Complexity:   ComplexityMedium,
			TaskSummary:  "Fix pagination bug",
			WorkflowType: "bugfix",
			SuggestedPhases: []PhaseSpec{
				{Name: "investigate", Description: "Find root cause"},
				{Name: "regression-tests", Description: "Write regression tests"},
				{Name: "fix", Description: "Apply fix"},
			},
			RelevantFiles: []FileRef{{Path: "paginate.go", Description: "pagination"}},
			Requirements:  []string{"Fix boundary check"},
			Approach:      "Adjust offset calculation",
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan error: %v", err)
	}
	if meta == nil {
		t.Fatal("GeneratePlan returned nil meta")
	}

	// Check WorkflowType
	if meta.WorkflowType != "bugfix" {
		t.Errorf("WorkflowType = %q, want %q", meta.WorkflowType, "bugfix")
	}

	// Check 3 phases
	if len(meta.Phases) != 3 {
		t.Errorf("Phases count = %d, want 3", len(meta.Phases))
	}

	// Each phase should have populated plan.md
	for _, phase := range meta.Phases {
		planMD, err := os.ReadFile(filepath.Join(tmpDir, "fix-bug", "phases", phase, "plan.md"))
		if err != nil {
			t.Fatalf("ReadFile plan.md for %s error: %v", phase, err)
		}
		if len(planMD) == 0 {
			t.Errorf("plan.md for %s is empty", phase)
		}
	}
}

func TestGeneratePlan_ComplexTaskWithProposal(t *testing.T) {
	tmpDir := t.TempDir()

	proposal := &ArchitectProposal{
		Name:       "pragmatic",
		Philosophy: "Balance speed and quality",
		SuggestedPhases: []PhaseSpec{
			{Name: "auth-types", Description: "Define auth types"},
			{Name: "auth-middleware", Description: "Add middleware"},
			{Name: "auth-routes", Description: "Wire routes"},
		},
		PlanContent: map[string]string{
			"auth-types":      "# Auth Types\n\nDefine the OAuth types and interfaces.",
			"auth-middleware":  "# Auth Middleware\n\nImplement authentication middleware.",
			"auth-routes":     "# Auth Routes\n\nWire up OAuth routes and sessions.",
		},
	}

	meta, err := GeneratePlan(GenerateOptions{
		PlanName: "add-auth",
		PlansDir: tmpDir,
		Discovery: &DiscoveryResult{
			Complexity:   ComplexityComplex,
			TaskSummary:  "Add OAuth authentication",
			WorkflowType: "custom",
			SuggestedPhases: []PhaseSpec{
				{Name: "auth-types", Description: "Define auth types"},
				{Name: "auth-middleware", Description: "Add middleware"},
				{Name: "auth-routes", Description: "Wire routes"},
			},
			RelevantFiles: []FileRef{},
			Requirements:  []string{},
			Approach:      "OAuth approach",
		},
		Proposal: proposal,
	})
	if err != nil {
		t.Fatalf("GeneratePlan error: %v", err)
	}
	if meta == nil {
		t.Fatal("GeneratePlan returned nil meta")
	}

	// Check custom workflow.yaml exists
	workflowPath := filepath.Join(tmpDir, "add-auth", "workflow.yaml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("workflow.yaml should exist for complex task: %v", err)
	}
	if len(workflowData) == 0 {
		t.Fatal("workflow.yaml should have content")
	}

	// Check 3 phases
	if len(meta.Phases) != 3 {
		t.Errorf("Phases count = %d, want 3", len(meta.Phases))
	}

	// Each phase's plan.md should match the proposal's PlanContent
	for phaseName, expectedContent := range proposal.PlanContent {
		planMD, err := os.ReadFile(filepath.Join(tmpDir, "add-auth", "phases", phaseName, "plan.md"))
		if err != nil {
			t.Fatalf("ReadFile plan.md for %s error: %v", phaseName, err)
		}
		if string(planMD) != expectedContent {
			t.Errorf("plan.md for %s = %q, want %q", phaseName, string(planMD), expectedContent)
		}
	}
}

func TestGeneratePlan_NilDiscovery(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := GeneratePlan(GenerateOptions{
		PlanName:  "test",
		PlansDir:  tmpDir,
		Discovery: nil,
	})
	if err == nil {
		t.Fatal("expected error for nil discovery, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "discovery") {
		t.Errorf("error = %q, want containing 'discovery'", err.Error())
	}
}

func TestGeneratePlan_EmptyPlanName(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := GeneratePlan(GenerateOptions{
		PlanName: "",
		PlansDir: tmpDir,
		Discovery: &DiscoveryResult{
			Complexity:      ComplexitySimple,
			TaskSummary:     "Test",
			WorkflowType:    "direct",
			SuggestedPhases: []PhaseSpec{{Name: "execute", Description: "do it"}},
			RelevantFiles:   []FileRef{},
			Requirements:    []string{},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty plan name, got nil")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "name") && !strings.Contains(errLower, "empty") {
		t.Errorf("error = %q, want containing 'name' or 'empty'", err.Error())
	}
}

func TestGeneratePlan_InvalidPlansDir(t *testing.T) {
	_, err := GeneratePlan(GenerateOptions{
		PlanName: "test",
		PlansDir: "/nonexistent/path/that/does/not/exist",
		Discovery: &DiscoveryResult{
			Complexity:      ComplexitySimple,
			TaskSummary:     "Test",
			WorkflowType:    "direct",
			SuggestedPhases: []PhaseSpec{{Name: "execute", Description: "do it"}},
			RelevantFiles:   []FileRef{},
			Requirements:    []string{},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid plans dir, got nil")
	}
}

func TestGeneratePlan_ComplexTaskWithoutProposal(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := GeneratePlan(GenerateOptions{
		PlanName: "complex-no-proposal",
		PlansDir: tmpDir,
		Discovery: &DiscoveryResult{
			Complexity:   ComplexityComplex,
			TaskSummary:  "Complex task",
			WorkflowType: "custom",
			SuggestedPhases: []PhaseSpec{
				{Name: "design", Description: "Design"},
				{Name: "impl", Description: "Implement"},
			},
			RelevantFiles: []FileRef{},
			Requirements:  []string{},
		},
		Proposal: nil,
	})
	if err == nil {
		t.Fatal("expected error for complex task without proposal, got nil")
	}
}

func TestGeneratePlan_PlanAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	discovery := &DiscoveryResult{
		Complexity:      ComplexitySimple,
		TaskSummary:     "Test",
		WorkflowType:    "direct",
		SuggestedPhases: []PhaseSpec{{Name: "execute", Description: "do it"}},
		RelevantFiles:   []FileRef{},
		Requirements:    []string{},
		Approach:        "Just do it",
	}

	// Create plan first time
	_, err := GeneratePlan(GenerateOptions{
		PlanName:  "existing",
		PlansDir:  tmpDir,
		Discovery: discovery,
	})
	if err != nil {
		t.Fatalf("first GeneratePlan error: %v", err)
	}

	// Try to create same plan again
	_, err = GeneratePlan(GenerateOptions{
		PlanName:  "existing",
		PlansDir:  tmpDir,
		Discovery: discovery,
	})
	if err == nil {
		t.Fatal("expected error for already-existing plan, got nil")
	}
}
