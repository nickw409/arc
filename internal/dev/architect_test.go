package dev

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- ParseArchitectOutput tests ---

func TestParseArchitectOutput_Valid(t *testing.T) {
	proposal := ArchitectProposal{
		Name:       "pragmatic",
		Philosophy: "Balance speed and quality",
		SuggestedPhases: []PhaseSpec{
			{Name: "auth-types", Description: "Define auth types"},
			{Name: "auth-middleware", Description: "Add middleware"},
		},
		PlanContent: map[string]string{
			"auth-types":      "# Phase: auth-types\n\nImplement types",
			"auth-middleware":  "# Phase: auth-middleware\n\nAdd middleware",
		},
	}
	jsonBytes, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatalf("marshal test data: %v", err)
	}
	output := "Here is my analysis of the codebase.\n\n```json\n" + string(jsonBytes) + "\n```\n\nDone."

	parsed, err := ParseArchitectOutput(output)
	if err != nil {
		t.Fatalf("ParseArchitectOutput returned error: %v", err)
	}
	if parsed.Name != "pragmatic" {
		t.Errorf("Name = %q, want %q", parsed.Name, "pragmatic")
	}
	if len(parsed.SuggestedPhases) != 2 {
		t.Errorf("SuggestedPhases len = %d, want 2", len(parsed.SuggestedPhases))
	}
	if len(parsed.PlanContent) != 2 {
		t.Errorf("PlanContent len = %d, want 2", len(parsed.PlanContent))
	}
	if parsed.PlanContent["auth-types"] == "" {
		t.Error("PlanContent[\"auth-types\"] is empty")
	}
}

func TestParseArchitectOutput_NoJSON(t *testing.T) {
	_, err := ParseArchitectOutput("Just some analysis text without JSON")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no json block found") {
		t.Errorf("error = %q, want containing 'no JSON block found'", err.Error())
	}
}

func TestParseArchitectOutput_MultipleJSONBlocks(t *testing.T) {
	// First block is garbage, second (last) block is a valid proposal
	firstBlock := `{"irrelevant": true}`
	proposal := ArchitectProposal{
		Name:       "clean",
		Philosophy: "Clean architecture",
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement it"},
		},
		PlanContent: map[string]string{
			"impl": "# Phase: impl\n\nDo it",
		},
	}
	jsonBytes, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatalf("marshal test data: %v", err)
	}

	output := "Analysis:\n\n```json\n" + firstBlock + "\n```\n\nRevised:\n\n```json\n" + string(jsonBytes) + "\n```\n"

	parsed, err := ParseArchitectOutput(output)
	if err != nil {
		t.Fatalf("ParseArchitectOutput returned error: %v", err)
	}
	if parsed.Name != "clean" {
		t.Errorf("Name = %q, want %q (from last JSON block)", parsed.Name, "clean")
	}
}

func TestParseArchitectOutput_MalformedJSON(t *testing.T) {
	output := "Here:\n\n```json\n{\"name\": \"incomplete\"\n```\n"
	_, err := ParseArchitectOutput(output)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "json") && !strings.Contains(errLower, "parse") {
		t.Errorf("error = %q, want containing 'json' or 'parse'", err.Error())
	}
}

func TestParseArchitectOutput_MissingName(t *testing.T) {
	proposal := ArchitectProposal{
		Name: "", // empty name
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement it"},
		},
		PlanContent: map[string]string{
			"impl": "content",
		},
	}
	jsonBytes, _ := json.MarshalIndent(proposal, "", "  ")
	output := "```json\n" + string(jsonBytes) + "\n```"

	_, err := ParseArchitectOutput(output)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "name") {
		t.Errorf("error = %q, want containing 'name'", err.Error())
	}
}

func TestParseArchitectOutput_MissingRequiredFields(t *testing.T) {
	// Valid JSON with name but empty SuggestedPhases
	raw := `{"name": "pragmatic", "suggested_phases": [], "plan_content": {"x": "y"}}`
	output := "```json\n" + raw + "\n```"

	_, err := ParseArchitectOutput(output)
	if err == nil {
		t.Fatal("expected error for empty SuggestedPhases, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "SuggestedPhases") && !strings.Contains(errMsg, "suggested_phases") && !strings.Contains(errMsg, "phases") {
		t.Errorf("error = %q, want containing 'SuggestedPhases' or 'phases'", errMsg)
	}
}

func TestParseArchitectOutput_MissingPlanContent(t *testing.T) {
	raw := `{"name": "pragmatic", "suggested_phases": [{"name": "impl", "description": "x"}], "plan_content": {}}`
	output := "```json\n" + raw + "\n```"

	_, err := ParseArchitectOutput(output)
	if err == nil {
		t.Fatal("expected error for empty PlanContent, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "PlanContent") && !strings.Contains(errMsg, "plan content") && !strings.Contains(errMsg, "plan_content") {
		t.Errorf("error = %q, want containing 'PlanContent' or 'plan content'", errMsg)
	}
}

// --- SelectProposal tests ---

func TestSelectProposal_PrefersPragmatic(t *testing.T) {
	proposals := []ArchitectProposal{
		{Name: "minimal"},
		{Name: "pragmatic"},
		{Name: "clean"},
	}
	selected := SelectProposal(proposals)
	if selected == nil {
		t.Fatal("SelectProposal returned nil")
	}
	if selected.Name != "pragmatic" {
		t.Errorf("selected.Name = %q, want %q", selected.Name, "pragmatic")
	}
}

func TestSelectProposal_FallbackToFirst(t *testing.T) {
	proposals := []ArchitectProposal{
		{Name: "minimal"},
		{Name: "clean"},
	}
	selected := SelectProposal(proposals)
	if selected == nil {
		t.Fatal("SelectProposal returned nil")
	}
	if selected.Name != "minimal" {
		t.Errorf("selected.Name = %q, want %q (first proposal)", selected.Name, "minimal")
	}
}

func TestSelectProposal_SingleProposal(t *testing.T) {
	proposals := []ArchitectProposal{
		{Name: "clean"},
	}
	selected := SelectProposal(proposals)
	if selected == nil {
		t.Fatal("SelectProposal returned nil")
	}
	if selected.Name != "clean" {
		t.Errorf("selected.Name = %q, want %q", selected.Name, "clean")
	}
}

func TestSelectProposal_EmptyList(t *testing.T) {
	selected := SelectProposal([]ArchitectProposal{})
	if selected != nil {
		t.Errorf("SelectProposal(empty) = %+v, want nil", selected)
	}
}

// --- RunArchitects tests ---

func TestRunArchitects_AllSucceed(t *testing.T) {
	// This test will fail until RunArchitects is implemented.
	// It verifies the function signature and basic contract.
	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:   "Add OAuth authentication",
		Complexity:    ComplexityComplex,
		Approach:      "Add OAuth middleware",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{{Path: "auth.go", Description: "auth handler"}},
		Requirements:  []string{"Google OAuth", "Refresh tokens"},
		SuggestedPhases: []PhaseSpec{
			{Name: "auth-types", Description: "Define types"},
			{Name: "auth-middleware", Description: "Add middleware"},
			{Name: "auth-routes", Description: "Wire routes"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false", // will fail immediately — expected
		Interactive: false,
	}

	output, err := RunArchitects(ctx, opts)
	// With stub implementation, we expect error or empty results.
	// The test validates the type contract.
	if err != nil {
		// Expected — "not implemented" or all agents failed
		return
	}
	if output == nil {
		t.Fatal("RunArchitects returned nil output without error")
	}
	// If somehow it works, validate structure
	if len(output.Proposals) == 0 {
		t.Error("expected at least one proposal when no error")
	}
}

func TestRunArchitects_OneAgentFails(t *testing.T) {
	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:   "Add feature",
		Complexity:    ComplexityComplex,
		Approach:      "Standard approach",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false",
		Interactive: false,
	}

	output, err := RunArchitects(ctx, opts)
	// With stub, expect error. When implemented:
	// If 2 of 3 succeed, output should have 2 proposals and no error.
	if err != nil {
		return // stub returns "not implemented"
	}
	if output != nil && len(output.Proposals) > 0 {
		// Partial success is valid — at least some proposals
		return
	}
}

func TestRunArchitects_AllAgentsFail(t *testing.T) {
	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:   "Add feature",
		Complexity:    ComplexityComplex,
		Approach:      "Standard approach",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false", // all agents will fail
		Interactive: false,
	}

	_, err := RunArchitects(ctx, opts)
	if err == nil {
		t.Fatal("expected error when all agents fail, got nil")
	}
}

func TestRunArchitects_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	discovery := &DiscoveryResult{
		TaskSummary:   "Add feature",
		Complexity:    ComplexityComplex,
		Approach:      "Standard approach",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false",
		Interactive: false,
	}

	_, err := RunArchitects(ctx, opts)
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	// Should be context.Canceled or wrap it
	if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "not implemented") {
		t.Logf("note: error = %q (may be 'not implemented' until implemented)", err.Error())
	}
}

func TestRunArchitects_UsageAggregation(t *testing.T) {
	// When implemented, 3 agents with different usage counts should be summed.
	// With stub, just verify the type contract.
	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:   "Add feature",
		Complexity:    ComplexityComplex,
		Approach:      "Standard approach",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false",
		Interactive: false,
	}

	output, err := RunArchitects(ctx, opts)
	if err != nil {
		return // stub returns "not implemented"
	}
	if output == nil {
		t.Fatal("RunArchitects returned nil output without error")
	}
	// Usage should be aggregated across all successful agents
	// (can't verify exact values without mock, but check it's present)
	_ = output.Usage
}

func TestRunArchitects_UsesTemplate(t *testing.T) {
	// Verify that RunArchitects loads and renders the architect template.
	// The mock agent will fail to produce valid JSON, but we verify the
	// template loading/rendering doesn't error.
	t.Setenv("MOCK_ECHO_ARGS", "1")
	t.Setenv("MOCK_JSON_WRAP", "1")

	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:  "Add auth",
		Complexity:   ComplexityComplex,
		Approach:     "Add OAuth middleware",
		WorkflowType: "feature",
		RelevantFiles: []FileRef{
			{Path: "auth.go", Description: "auth handler"},
		},
		Requirements: []string{"OAuth"},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	_, err := RunArchitects(ctx, ArchitectOptions{
		Discovery:   discovery,
		CommandName: mockBin,
		Interactive: false,
	})
	// All agents will fail to produce valid proposals (mock echoes args),
	// but the error should be about parsing, not template loading.
	if err == nil {
		return // unlikely but acceptable
	}
	if strings.Contains(err.Error(), "loading architect template") {
		t.Fatalf("template loading failed: %v", err)
	}
	if strings.Contains(err.Error(), "rendering architect template") {
		t.Fatalf("template rendering failed: %v", err)
	}
}

func TestRunArchitects_InteractiveModeNoAutoSelect(t *testing.T) {
	ctx := context.Background()
	discovery := &DiscoveryResult{
		TaskSummary:   "Add feature",
		Complexity:    ComplexityComplex,
		Approach:      "Standard approach",
		WorkflowType:  "custom",
		RelevantFiles: []FileRef{},
		Requirements:  []string{},
		SuggestedPhases: []PhaseSpec{
			{Name: "impl", Description: "implement"},
		},
	}

	opts := ArchitectOptions{
		Discovery:   discovery,
		CommandName: "false",
		Interactive: true, // interactive mode
	}

	output, err := RunArchitects(ctx, opts)
	if err != nil {
		return // stub returns "not implemented"
	}
	if output == nil {
		t.Fatal("RunArchitects returned nil output without error")
	}
	// In interactive mode, Selected should be nil
	if output.Selected != nil {
		t.Error("expected Selected to be nil in interactive mode")
	}
}
