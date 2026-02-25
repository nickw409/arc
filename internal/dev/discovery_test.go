package dev

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var mockBin string

func TestMain(m *testing.M) {
	// Build the mock agent binary for RunDiscovery tests.
	tmpDir, err := os.MkdirTemp("", "dev-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	mockBin = filepath.Join(tmpDir, "mockagent")
	cmd := exec.Command("go", "build", "-o", mockBin, "../agent/testdata/mockagent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build mock agent: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// ParseDiscoveryOutput tests
// ---------------------------------------------------------------------------

func TestParseDiscoveryOutput_ValidJSON(t *testing.T) {
	input := "Here is my analysis of the codebase.\n\n## Analysis\n\nThe task involves fixing a typo in the README.\n\n" +
		"```json\n" +
		`{"task_summary": "Fix typo in README.md", "complexity": "simple", "reasoning": "Single file change, obvious fix", "relevant_files": [{"path": "README.md", "description": "Contains the typo"}], "requirements": ["Fix spelling of 'authentication'"], "approach": "Direct edit to README.md", "workflow_type": "direct", "suggested_phases": [{"name": "execute", "description": "Fix the typo"}]}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TaskSummary != "Fix typo in README.md" {
		t.Errorf("TaskSummary = %q, want %q", result.TaskSummary, "Fix typo in README.md")
	}
	if result.Complexity != ComplexitySimple {
		t.Errorf("Complexity = %q, want %q", result.Complexity, ComplexitySimple)
	}
	if result.WorkflowType != "direct" {
		t.Errorf("WorkflowType = %q, want %q", result.WorkflowType, "direct")
	}
	if len(result.RelevantFiles) != 1 {
		t.Errorf("len(RelevantFiles) = %d, want 1", len(result.RelevantFiles))
	}
	if len(result.SuggestedPhases) != 1 {
		t.Errorf("len(SuggestedPhases) = %d, want 1", len(result.SuggestedPhases))
	}
}

func TestParseDiscoveryOutput_ComplexTask(t *testing.T) {
	input := "```json\n" +
		`{
  "task_summary": "Add OAuth authentication to the API",
  "complexity": "complex",
  "reasoning": "Touches 8+ files, needs auth architecture decisions",
  "relevant_files": [
    {"path": "internal/auth/handler.go", "description": "existing auth"},
    {"path": "internal/middleware/auth.go", "description": "auth middleware"},
    {"path": "internal/config/config.go", "description": "config parsing"},
    {"path": "cmd/server/main.go", "description": "server entry point"},
    {"path": "internal/models/user.go", "description": "user model"},
    {"path": "internal/routes/routes.go", "description": "route definitions"}
  ],
  "requirements": ["Support Google OAuth", "Support GitHub OAuth", "Refresh tokens", "Session management"],
  "approach": "Add OAuth middleware layer with provider abstraction",
  "workflow_type": "feature",
  "suggested_phases": [
    {"name": "oauth-types", "description": "Define OAuth types and interfaces"},
    {"name": "oauth-providers", "description": "Implement Google and GitHub providers"},
    {"name": "oauth-middleware", "description": "Add authentication middleware"},
    {"name": "oauth-routes", "description": "Wire up OAuth routes and sessions"}
  ]
}` + "\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Complexity != ComplexityComplex {
		t.Errorf("Complexity = %q, want %q", result.Complexity, ComplexityComplex)
	}
	if len(result.RelevantFiles) != 6 {
		t.Errorf("len(RelevantFiles) = %d, want 6", len(result.RelevantFiles))
	}
	if len(result.SuggestedPhases) != 4 {
		t.Errorf("len(SuggestedPhases) = %d, want 4", len(result.SuggestedPhases))
	}
}

func TestParseDiscoveryOutput_NoJSONBlock(t *testing.T) {
	input := "This is just some text without any JSON block"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for missing JSON block")
	}
	if !strings.Contains(err.Error(), "no JSON block found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "no JSON block found")
	}
}

func TestParseDiscoveryOutput_InvalidJSON(t *testing.T) {
	input := "```json\n{invalid json here}\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unmarshal") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unmarshal")
	}
}

func TestParseDiscoveryOutput_MissingTaskSummary(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "", "complexity": "simple", "workflow_type": "direct"}` +
		"\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for empty task_summary")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "task_summary") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "task_summary")
	}
}

func TestParseDiscoveryOutput_InvalidComplexity(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "do stuff", "complexity": "extreme", "workflow_type": "direct"}` +
		"\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for invalid complexity")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "complexity") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "complexity")
	}
}

func TestParseDiscoveryOutput_MissingWorkflowType(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "fix bug", "complexity": "simple", "workflow_type": ""}` +
		"\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for empty workflow_type")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "workflow_type") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "workflow_type")
	}
}

func TestParseDiscoveryOutput_CaseInsensitiveJSONMarker(t *testing.T) {
	input := "Some preamble text.\n\n" +
		"```JSON\n" +
		`{"task_summary": "fix bug", "complexity": "simple", "reasoning": "small", "workflow_type": "direct"}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TaskSummary != "fix bug" {
		t.Errorf("TaskSummary = %q, want %q", result.TaskSummary, "fix bug")
	}
}

func TestParseDiscoveryOutput_NilSlicesInitialized(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "fix bug", "complexity": "simple", "reasoning": "small", "workflow_type": "direct"}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RelevantFiles == nil {
		t.Error("RelevantFiles is nil, want empty slice")
	}
	if len(result.RelevantFiles) != 0 {
		t.Errorf("len(RelevantFiles) = %d, want 0", len(result.RelevantFiles))
	}
	if result.Requirements == nil {
		t.Error("Requirements is nil, want empty slice")
	}
	if len(result.Requirements) != 0 {
		t.Errorf("len(Requirements) = %d, want 0", len(result.Requirements))
	}
	if result.SuggestedPhases == nil {
		t.Error("SuggestedPhases is nil, want empty slice")
	}
	if len(result.SuggestedPhases) != 0 {
		t.Errorf("len(SuggestedPhases) = %d, want 0", len(result.SuggestedPhases))
	}
}

func TestParseDiscoveryOutput_MultipleJSONBlocks(t *testing.T) {
	input := "First block:\n" +
		"```json\n" +
		`{"task_summary": "first block", "complexity": "simple", "reasoning": "first", "workflow_type": "direct"}` +
		"\n```\n\n" +
		"Second block:\n" +
		"```json\n" +
		`{"task_summary": "second block", "complexity": "complex", "reasoning": "second", "workflow_type": "feature"}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Per spec: parses the FIRST complete ```json block
	if result.TaskSummary != "first block" {
		t.Errorf("TaskSummary = %q, want %q (should use first JSON block)", result.TaskSummary, "first block")
	}
}

func TestParseDiscoveryOutput_IncompleteJSONBlock(t *testing.T) {
	// Opening ```json fence but no closing ``` fence
	input := "```json\n{\"task_summary\": \"incomplete\", \"complexity\": \"simple\""

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for incomplete JSON block (no closing fence)")
	}
}

func TestParseDiscoveryOutput_EmptyJSONObject(t *testing.T) {
	input := "```json\n{}\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for empty JSON object")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "task_summary") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "task_summary")
	}
}

func TestParseDiscoveryOutput_WhitespaceOnlyTaskSummary(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "   ", "complexity": "simple", "workflow_type": "direct"}` +
		"\n```"

	_, err := ParseDiscoveryOutput(input)
	if err == nil {
		t.Fatal("expected error for whitespace-only task_summary")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "task_summary") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "task_summary")
	}
}

func TestParseDiscoveryOutput_NestedCodeFences(t *testing.T) {
	// Agent wraps JSON in nested code fences — parser should still extract the JSON
	input := "Outer text\n```\n" +
		"```json\n" +
		`{"task_summary": "nested", "complexity": "simple", "reasoning": "test", "workflow_type": "direct"}` +
		"\n```\n" +
		"```\nMore text"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TaskSummary != "nested" {
		t.Errorf("TaskSummary = %q, want %q", result.TaskSummary, "nested")
	}
}

// ---------------------------------------------------------------------------
// ValidateComplexity tests
// ---------------------------------------------------------------------------

func TestValidateComplexity_SimpleWithManyFiles(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:    ComplexitySimple,
		RelevantFiles: make([]FileRef, 6),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (>5 files should upgrade simple to medium)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_SimpleWithMultiplePhases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		SuggestedPhases: make([]PhaseSpec, 2),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (>1 phase should upgrade simple to medium)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_MediumWithManyPhases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityMedium,
		SuggestedPhases: make([]PhaseSpec, 5),
	}
	got := ValidateComplexity(result)
	if got != ComplexityComplex {
		t.Errorf("ValidateComplexity = %q, want %q (>4 phases should upgrade medium to complex)", got, ComplexityComplex)
	}
}

func TestValidateComplexity_ComplexDowngradeToSimple(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityComplex,
		RelevantFiles:  make([]FileRef, 1),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexitySimple {
		t.Errorf("ValidateComplexity = %q, want %q (<=2 files and <=1 phase should downgrade complex to simple)", got, ComplexitySimple)
	}
}

func TestValidateComplexity_NoOverrideNeeded(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityMedium,
		RelevantFiles:  make([]FileRef, 3),
		SuggestedPhases: make([]PhaseSpec, 3),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (no override needed)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_BoundaryExactly5Files(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 5),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexitySimple {
		t.Errorf("ValidateComplexity = %q, want %q (exactly 5 files does not trigger >5 override)", got, ComplexitySimple)
	}
}

func TestValidateComplexity_BoundaryExactly6Files(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 6),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (>5 files triggers override)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_BoundaryExactly1Phase(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 2),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexitySimple {
		t.Errorf("ValidateComplexity = %q, want %q (exactly 1 phase does not trigger >1 override)", got, ComplexitySimple)
	}
}

func TestValidateComplexity_BoundaryExactly2Phases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 2),
		SuggestedPhases: make([]PhaseSpec, 2),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (>1 phase triggers override)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_BoundaryExactly4Phases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityMedium,
		RelevantFiles:  make([]FileRef, 3),
		SuggestedPhases: make([]PhaseSpec, 4),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (exactly 4 phases does not trigger >4 override)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_BoundaryExactly5Phases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityMedium,
		RelevantFiles:  make([]FileRef, 3),
		SuggestedPhases: make([]PhaseSpec, 5),
	}
	got := ValidateComplexity(result)
	if got != ComplexityComplex {
		t.Errorf("ValidateComplexity = %q, want %q (>4 phases triggers override)", got, ComplexityComplex)
	}
}

func TestValidateComplexity_ComplexDowngradeWith2Files(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityComplex,
		RelevantFiles:  make([]FileRef, 2),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexitySimple {
		t.Errorf("ValidateComplexity = %q, want %q (<=2 files and <=1 phase triggers downgrade)", got, ComplexitySimple)
	}
}

func TestValidateComplexity_NilResult(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil input, but did not panic")
		}
	}()
	ValidateComplexity(nil)
}

func TestValidateComplexity_ZeroPhases(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 2),
		SuggestedPhases: []PhaseSpec{},
	}
	got := ValidateComplexity(result)
	if got != ComplexitySimple {
		t.Errorf("ValidateComplexity = %q, want %q (0 phases does not trigger >1 override)", got, ComplexitySimple)
	}
}

func TestValidateComplexity_CombinedOverrides(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexitySimple,
		RelevantFiles:  make([]FileRef, 6),
		SuggestedPhases: make([]PhaseSpec, 2),
	}
	got := ValidateComplexity(result)
	if got != ComplexityMedium {
		t.Errorf("ValidateComplexity = %q, want %q (both >5 files and >1 phase, upgrade to medium)", got, ComplexityMedium)
	}
}

func TestValidateComplexity_NoDowngradeAt3Files(t *testing.T) {
	result := &DiscoveryResult{
		Complexity:     ComplexityComplex,
		RelevantFiles:  make([]FileRef, 3),
		SuggestedPhases: make([]PhaseSpec, 1),
	}
	got := ValidateComplexity(result)
	if got != ComplexityComplex {
		t.Errorf("ValidateComplexity = %q, want %q (3 files is >2, so no downgrade)", got, ComplexityComplex)
	}
}

// ---------------------------------------------------------------------------
// ValidateWorkflowType tests
// ---------------------------------------------------------------------------

func TestValidateWorkflowType_Valid(t *testing.T) {
	got := ValidateWorkflowType("bugfix")
	if got != "bugfix" {
		t.Errorf("ValidateWorkflowType(%q) = %q, want %q", "bugfix", got, "bugfix")
	}
}

func TestValidateWorkflowType_Direct(t *testing.T) {
	got := ValidateWorkflowType("direct")
	if got != "direct" {
		t.Errorf("ValidateWorkflowType(%q) = %q, want %q", "direct", got, "direct")
	}
}

func TestValidateWorkflowType_Unknown(t *testing.T) {
	got := ValidateWorkflowType("unknown_type")
	if got != "feature" {
		t.Errorf("ValidateWorkflowType(%q) = %q, want %q (default fallback)", "unknown_type", got, "feature")
	}
}

func TestValidateWorkflowType_AllValidTypes(t *testing.T) {
	validTypes := []string{"feature", "bugfix", "investigation", "refactor", "performance", "direct", "audit"}
	for _, wt := range validTypes {
		got := ValidateWorkflowType(wt)
		if got != wt {
			t.Errorf("ValidateWorkflowType(%q) = %q, want %q", wt, got, wt)
		}
	}
}

func TestValidateWorkflowType_Empty(t *testing.T) {
	got := ValidateWorkflowType("")
	if got != "feature" {
		t.Errorf("ValidateWorkflowType(%q) = %q, want %q (default fallback)", "", got, "feature")
	}
}

// ---------------------------------------------------------------------------
// RunDiscovery tests
// ---------------------------------------------------------------------------

// validAgentOutput returns agent text containing a valid ```json discovery result.
func validAgentOutput() string {
	return "Analysis complete.\n" +
		"```json\n" +
		`{"task_summary":"fix typo","complexity":"simple","reasoning":"test","workflow_type":"direct","relevant_files":[],"requirements":[],"suggested_phases":[]}` +
		"\n```"
}

func TestRunDiscovery_WiringSpawnsAgent(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "You are a discovery agent.",
		CommandName:     mockBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
	if output.Result.TaskSummary != "fix typo" {
		t.Errorf("Result.TaskSummary = %q, want %q", output.Result.TaskSummary, "fix typo")
	}
	if output.Result.Complexity != ComplexitySimple {
		t.Errorf("Result.Complexity = %q, want %q", output.Result.Complexity, ComplexitySimple)
	}
	if output.Usage.IsZero() {
		t.Error("expected non-zero Usage")
	}
	if output.Raw == "" {
		t.Error("expected non-empty Raw")
	}
}

func TestRunDiscovery_AgentTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := RunDiscovery(ctx, DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "You are a discovery agent.",
		CommandName:     mockBin,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRunDiscovery_InvalidAgentOutput(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "just some text without any JSON block")
	t.Setenv("MOCK_JSON_WRAP", "1")

	_, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "You are a discovery agent.",
		CommandName:     mockBin,
	})
	if err == nil {
		t.Fatal("expected error for invalid agent output (no JSON block)")
	}
}

func TestRunDiscovery_AgentBinaryNotFound(t *testing.T) {
	_, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "You are a discovery agent.",
		CommandName:     "/nonexistent/binary",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestRunDiscovery_EmptyPrompt(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "",
		CommandName:     mockBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
}

func TestRunDiscovery_EmptyTaskDescription(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	// Empty task description should either succeed or return a validation error.
	// Document whichever behavior the implementation chooses.
	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "",
		Prompt:          "You are a discovery agent.",
		CommandName:     mockBin,
	})
	if err != nil {
		// Acceptable: returns validation error for empty task
		return
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput when no error returned")
	}
}

func TestRunDiscovery_DefaultCommandName(t *testing.T) {
	// Create a temp dir with a "claude" symlink pointing at the mock binary,
	// then prepend that dir to PATH so agent.Spawn resolves "claude" to our mock.
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "claude")
	if err := os.Symlink(mockBin, claudePath); err != nil {
		t.Fatalf("failed to symlink mock binary as claude: %v", err)
	}

	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "You are a discovery agent.",
		CommandName:     "", // empty — should default to "claude"
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
	if output.Result.TaskSummary != "fix typo" {
		t.Errorf("Result.TaskSummary = %q, want %q", output.Result.TaskSummary, "fix typo")
	}
}

func TestRunDiscovery_UsesTemplateWhenNoPrompt(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")
	t.Setenv("MOCK_ECHO_ARGS", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "add user auth",
		Prompt:          "", // empty — should use template
		CommandName:     mockBin,
	})
	if err != nil {
		// The mock echoes args, which won't contain valid JSON.
		// We just need to verify the template was loaded — check that the
		// error is about parsing, not about loading the template.
		if strings.Contains(err.Error(), "loading discovery template") {
			t.Fatalf("template loading failed: %v", err)
		}
		if strings.Contains(err.Error(), "rendering discovery template") {
			t.Fatalf("template rendering failed: %v", err)
		}
		// Parse error from mock output is expected — the template was rendered.
		return
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
}

func TestRunDiscovery_ExplicitPromptOverridesTemplate(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "fix typo",
		Prompt:          "Custom prompt override.",
		CommandName:     mockBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
	if output.Result.TaskSummary != "fix typo" {
		t.Errorf("Result.TaskSummary = %q, want %q", output.Result.TaskSummary, "fix typo")
	}
}

func TestParseDiscoveryOutput_OptionalFields(t *testing.T) {
	input := "```json\n" +
		`{
  "task_summary": "Add auth",
  "complexity": "medium",
  "reasoning": "Multiple files",
  "workflow_type": "feature",
  "relevant_files": [{"path": "auth.go", "description": "auth handler"}],
  "requirements": ["OAuth support"],
  "approach": "Add middleware",
  "suggested_phases": [{"name": "impl", "description": "implement"}],
  "dependencies": {"auth.go": ["net/http", "context"]},
  "conventions": ["errors wrapped with fmt.Errorf"],
  "risks": ["breaking change to auth middleware"]
}` + "\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Dependencies) != 1 {
		t.Errorf("len(Dependencies) = %d, want 1", len(result.Dependencies))
	}
	if deps, ok := result.Dependencies["auth.go"]; !ok || len(deps) != 2 {
		t.Errorf("Dependencies[\"auth.go\"] = %v, want [net/http context]", deps)
	}
	if len(result.Conventions) != 1 {
		t.Errorf("len(Conventions) = %d, want 1", len(result.Conventions))
	}
	if len(result.Risks) != 1 {
		t.Errorf("len(Risks) = %d, want 1", len(result.Risks))
	}
}

func TestParseDiscoveryOutput_WithQuestions(t *testing.T) {
	input := "```json\n" +
		`{
  "task_summary": "Add OAuth",
  "complexity": "medium",
  "reasoning": "Multiple files",
  "workflow_type": "feature",
  "relevant_files": [],
  "requirements": [],
  "suggested_phases": [{"name": "impl", "description": "implement"}],
  "questions": ["Which providers?", "JWT or sessions?"]
}` + "\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Questions) != 2 {
		t.Fatalf("len(Questions) = %d, want 2", len(result.Questions))
	}
	if result.Questions[0] != "Which providers?" {
		t.Errorf("Questions[0] = %q, want %q", result.Questions[0], "Which providers?")
	}
	if result.Questions[1] != "JWT or sessions?" {
		t.Errorf("Questions[1] = %q, want %q", result.Questions[1], "JWT or sessions?")
	}
}

func TestParseDiscoveryOutput_NoQuestions(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "fix bug", "complexity": "simple", "reasoning": "small", "workflow_type": "direct"}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Questions == nil {
		t.Error("Questions is nil, want empty slice")
	}
	if len(result.Questions) != 0 {
		t.Errorf("len(Questions) = %d, want 0", len(result.Questions))
	}
}

func TestParseDiscoveryOutput_OptionalFieldsAbsent(t *testing.T) {
	input := "```json\n" +
		`{"task_summary": "fix bug", "complexity": "simple", "reasoning": "small", "workflow_type": "direct"}` +
		"\n```"

	result, err := ParseDiscoveryOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Dependencies != nil {
		t.Errorf("Dependencies = %v, want nil", result.Dependencies)
	}
	if result.Conventions == nil {
		t.Error("Conventions is nil, want empty slice")
	}
	if result.Risks == nil {
		t.Error("Risks is nil, want empty slice")
	}
}

func TestRunDiscovery_ModelOverride(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", validAgentOutput())
	t.Setenv("MOCK_JSON_WRAP", "1")

	output, err := RunDiscovery(context.Background(), DiscoveryOptions{
		TaskDescription: "task",
		Prompt:          "You are a discovery agent.",
		Model:           "opus",
		CommandName:     mockBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil DiscoveryOutput")
	}
	if output.Result.TaskSummary != "fix typo" {
		t.Errorf("Result.TaskSummary = %q, want %q", output.Result.TaskSummary, "fix typo")
	}
}
