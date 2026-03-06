package dev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
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

	// Complex tasks use a standard per-phase workflow type, not a monolithic
	// custom workflow. No workflow.yaml should be written at the plan level.
	workflowPath := filepath.Join(tmpDir, "add-auth", "workflow.yaml")
	if _, err := os.Stat(workflowPath); err == nil {
		t.Fatal("workflow.yaml should NOT exist for complex tasks: each phase uses a standard workflow type")
	}

	// WorkflowType should be a standard type, not "custom"
	if meta.WorkflowType == "custom" {
		t.Errorf("WorkflowType = %q, want a standard type like 'feature' — complex tasks must not use a monolithic custom workflow", meta.WorkflowType)
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

// --- Clarifications in plan.md tests ---

func TestGenerateSimplePlanMD_WithClarifications(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:  "Add OAuth authentication",
		Approach:     "Token middleware",
		Complexity:   ComplexitySimple,
		WorkflowType: "direct",
		Clarifications: []Clarification{
			{Question: "Which providers?", Answer: "Google and GitHub"},
			{Question: "JWT or sessions?", Answer: "JWTs"},
		},
	}

	result := GenerateSimplePlanMD(discovery)

	if !strings.Contains(result, "## Clarifications") {
		t.Errorf("expected Clarifications section, got:\n%s", result)
	}
	if !strings.Contains(result, "Which providers?") {
		t.Errorf("expected question text, got:\n%s", result)
	}
	if !strings.Contains(result, "Google and GitHub") {
		t.Errorf("expected answer text, got:\n%s", result)
	}
	if !strings.Contains(result, "JWTs") {
		t.Errorf("expected second answer, got:\n%s", result)
	}
}

func TestGeneratePhasePlanMD_WithClarifications(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:  "Add OAuth",
		Complexity:   ComplexityMedium,
		WorkflowType: "feature",
		Clarifications: []Clarification{
			{Question: "Which DB?", Answer: "Postgres"},
		},
	}
	phase := PhaseSpec{Name: "impl", Description: "Implement it"}

	result := GeneratePhasePlanMD(discovery, phase)

	if !strings.Contains(result, "## Clarifications") {
		t.Errorf("expected Clarifications section, got:\n%s", result)
	}
	if !strings.Contains(result, "Which DB?") {
		t.Errorf("expected question text, got:\n%s", result)
	}
	if !strings.Contains(result, "Postgres") {
		t.Errorf("expected answer text, got:\n%s", result)
	}
}

func TestGenerateSimplePlanMD_NoClarifications(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:    "Fix typo",
		Complexity:     ComplexitySimple,
		WorkflowType:   "direct",
		Clarifications: []Clarification{},
	}

	result := GenerateSimplePlanMD(discovery)

	if strings.Contains(result, "## Clarifications") {
		t.Errorf("expected no Clarifications section when empty, got:\n%s", result)
	}
}

func TestGeneratePhasePlanMD_NoClarifications(t *testing.T) {
	discovery := &DiscoveryResult{
		TaskSummary:    "Fix bug",
		Complexity:     ComplexityMedium,
		WorkflowType:   "bugfix",
		Clarifications: nil,
	}
	phase := PhaseSpec{Name: "fix", Description: "Fix it"}

	result := GeneratePhasePlanMD(discovery, phase)

	if strings.Contains(result, "## Clarifications") {
		t.Errorf("expected no Clarifications section when nil, got:\n%s", result)
	}
}
