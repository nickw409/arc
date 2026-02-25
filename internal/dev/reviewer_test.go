package dev

import (
	"context"
	"strings"
	"testing"
)

func TestParseReviewOutput_Valid(t *testing.T) {
	output := "Here is my review.\n\n```json\n" +
		`{
  "issues": [
    {
      "severity": "warning",
      "file": "auth.go",
      "line": 42,
      "description": "Missing nil check on user",
      "suggestion": "Add if user == nil check before accessing fields"
    },
    {
      "severity": "suggestion",
      "file": "auth_test.go",
      "description": "Test only covers happy path"
    }
  ],
  "summary": "Generally good implementation with minor issues"
}` + "\n```\nDone."

	result, err := ParseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("len(Issues) = %d, want 2", len(result.Issues))
	}
	if result.Issues[0].Severity != "warning" {
		t.Errorf("Issues[0].Severity = %q, want %q", result.Issues[0].Severity, "warning")
	}
	if result.Issues[0].File != "auth.go" {
		t.Errorf("Issues[0].File = %q, want %q", result.Issues[0].File, "auth.go")
	}
	if result.Issues[0].Line != 42 {
		t.Errorf("Issues[0].Line = %d, want 42", result.Issues[0].Line)
	}
	if result.Issues[1].Line != 0 {
		t.Errorf("Issues[1].Line = %d, want 0 (omitted)", result.Issues[1].Line)
	}
	if result.Summary != "Generally good implementation with minor issues" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Generally good implementation with minor issues")
	}
}

func TestParseReviewOutput_NoIssues(t *testing.T) {
	output := "```json\n" +
		`{"issues": [], "summary": "Code looks great, no issues found"}` +
		"\n```"

	result, err := ParseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0", len(result.Issues))
	}
	if result.Summary == "" {
		t.Error("Summary is empty, expected non-empty")
	}
}

func TestParseReviewOutput_NullIssues(t *testing.T) {
	output := "```json\n" +
		`{"issues": null, "summary": "All good"}` +
		"\n```"

	result, err := ParseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Issues == nil {
		t.Error("Issues is nil, want empty slice")
	}
	if len(result.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0", len(result.Issues))
	}
}

func TestParseReviewOutput_NoJSON(t *testing.T) {
	output := "This is just text without any JSON block."

	_, err := ParseReviewOutput(output)
	if err == nil {
		t.Fatal("expected error for missing JSON block")
	}
	if !strings.Contains(err.Error(), "no JSON block found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "no JSON block found")
	}
}

func TestParseReviewOutput_InvalidJSON(t *testing.T) {
	output := "```json\n{invalid json}\n```"

	_, err := ParseReviewOutput(output)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "json") {
		t.Errorf("error = %q, want it to contain 'json'", err.Error())
	}
}

func TestParseReviewOutput_MultipleJSONBlocks(t *testing.T) {
	output := "First attempt:\n```json\n{\"bad\": true}\n```\n\n" +
		"Revised:\n```json\n" +
		`{"issues": [{"severity": "critical", "file": "main.go", "description": "SQL injection"}], "summary": "Found critical issue"}` +
		"\n```"

	result, err := ParseReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use the last JSON block
	if len(result.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(result.Issues))
	}
	if result.Issues[0].Severity != "critical" {
		t.Errorf("Issues[0].Severity = %q, want %q", result.Issues[0].Severity, "critical")
	}
}

func TestRunCodeReview_Wiring(t *testing.T) {
	t.Setenv("MOCK_ECHO_ARGS", "1")
	t.Setenv("MOCK_JSON_WRAP", "1")

	_, err := RunCodeReview(context.Background(), ReviewOptions{
		PlanDir:     t.TempDir(),
		ProjectDir:  t.TempDir(),
		Diff:        "+added line\n-removed line",
		PlanMD:      "# Test Plan\n\nDo the thing.",
		CommandName: mockBin,
	})
	// Mock echoes args, which won't be valid review JSON.
	// Verify the error is about parsing, not template loading.
	if err == nil {
		return // acceptable if mock happens to produce valid output
	}
	if strings.Contains(err.Error(), "loading reviewer template") {
		t.Fatalf("template loading failed: %v", err)
	}
	if strings.Contains(err.Error(), "rendering reviewer template") {
		t.Fatalf("template rendering failed: %v", err)
	}
}

func TestRunCodeReview_ValidOutput(t *testing.T) {
	reviewJSON := `{"issues": [{"severity": "warning", "file": "main.go", "description": "unused import"}], "summary": "Minor issue"}`
	output := "Review complete.\n```json\n" + reviewJSON + "\n```"

	t.Setenv("MOCK_OUTPUT", output)
	t.Setenv("MOCK_JSON_WRAP", "1")

	result, err := RunCodeReview(context.Background(), ReviewOptions{
		PlanDir:     t.TempDir(),
		ProjectDir:  t.TempDir(),
		Diff:        "+line",
		PlanMD:      "plan",
		CommandName: mockBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(result.Issues))
	}
	if result.Issues[0].Severity != "warning" {
		t.Errorf("Issues[0].Severity = %q, want %q", result.Issues[0].Severity, "warning")
	}
	if result.Usage.IsZero() {
		t.Error("expected non-zero Usage")
	}
}
