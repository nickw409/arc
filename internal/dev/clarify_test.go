package dev

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ShouldAskQuestions tests
// ---------------------------------------------------------------------------

func TestShouldAskQuestions_SimpleNoQuestions(t *testing.T) {
	if ShouldAskQuestions(ComplexitySimple, nil) {
		t.Error("ShouldAskQuestions(simple, nil) = true, want false")
	}
}

func TestShouldAskQuestions_MediumWithQuestions(t *testing.T) {
	if !ShouldAskQuestions(ComplexityMedium, []string{"Which approach?"}) {
		t.Error("ShouldAskQuestions(medium, [q]) = false, want true")
	}
}

func TestShouldAskQuestions_ComplexWithQuestions(t *testing.T) {
	if !ShouldAskQuestions(ComplexityComplex, []string{"Which pattern?", "Which DB?"}) {
		t.Error("ShouldAskQuestions(complex, [q, q]) = false, want true")
	}
}

func TestShouldAskQuestions_MediumNoQuestions(t *testing.T) {
	if ShouldAskQuestions(ComplexityMedium, []string{}) {
		t.Error("ShouldAskQuestions(medium, []) = true, want false")
	}
}

func TestShouldAskQuestions_SimpleWithQuestions(t *testing.T) {
	if ShouldAskQuestions(ComplexitySimple, []string{"Some question?"}) {
		t.Error("ShouldAskQuestions(simple, [q]) = true, want false (simple tasks skip questions)")
	}
}

// ---------------------------------------------------------------------------
// FormatDiscoverySummary tests
// ---------------------------------------------------------------------------

func TestFormatDiscoverySummary_Basic(t *testing.T) {
	d := &DiscoveryResult{
		TaskSummary:  "Add OAuth authentication",
		Complexity:   ComplexityMedium,
		WorkflowType: "feature",
		Approach:     "Token refresh middleware with provider abstraction",
	}

	result := FormatDiscoverySummary(d)

	checks := []string{
		"Add OAuth authentication",
		"medium",
		"feature workflow",
		"Token refresh middleware",
	}
	for _, c := range checks {
		if !strings.Contains(result, c) {
			t.Errorf("FormatDiscoverySummary missing %q, got:\n%s", c, result)
		}
	}
}

func TestFormatDiscoverySummary_WithPhases(t *testing.T) {
	d := &DiscoveryResult{
		TaskSummary:  "Add auth",
		Complexity:   ComplexityMedium,
		WorkflowType: "feature",
		SuggestedPhases: []PhaseSpec{
			{Name: "oauth-types"},
			{Name: "oauth-middleware"},
			{Name: "oauth-routes"},
		},
	}

	result := FormatDiscoverySummary(d)

	if !strings.Contains(result, "oauth-types, oauth-middleware, oauth-routes") {
		t.Errorf("FormatDiscoverySummary missing phase names, got:\n%s", result)
	}
}

func TestFormatDiscoverySummary_EmptyOptionals(t *testing.T) {
	d := &DiscoveryResult{
		TaskSummary:     "Minimal task",
		Complexity:      ComplexitySimple,
		SuggestedPhases: []PhaseSpec{},
	}

	// Should not panic
	result := FormatDiscoverySummary(d)

	if !strings.Contains(result, "Minimal task") {
		t.Errorf("FormatDiscoverySummary missing task summary, got:\n%s", result)
	}
	if strings.Contains(result, "Phases:") {
		t.Errorf("FormatDiscoverySummary should not include Phases line when empty, got:\n%s", result)
	}
	if strings.Contains(result, "Approach:") {
		t.Errorf("FormatDiscoverySummary should not include Approach line when empty, got:\n%s", result)
	}
}

// ---------------------------------------------------------------------------
// AskQuestions tests
// ---------------------------------------------------------------------------

func TestAskQuestions_AllAnswered(t *testing.T) {
	input := "Both Google and GitHub\nJWTs with refresh tokens\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	questions := []string{
		"Which OAuth providers?",
		"JWT or server-side sessions?",
	}

	clarifications, err := AskQuestions(questions, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clarifications) != 2 {
		t.Fatalf("len(clarifications) = %d, want 2", len(clarifications))
	}

	if clarifications[0].Question != "Which OAuth providers?" {
		t.Errorf("clarifications[0].Question = %q, want %q", clarifications[0].Question, "Which OAuth providers?")
	}
	if clarifications[0].Answer != "Both Google and GitHub" {
		t.Errorf("clarifications[0].Answer = %q, want %q", clarifications[0].Answer, "Both Google and GitHub")
	}
	if clarifications[1].Answer != "JWTs with refresh tokens" {
		t.Errorf("clarifications[1].Answer = %q, want %q", clarifications[1].Answer, "JWTs with refresh tokens")
	}
}

func TestAskQuestions_SkippedQuestion(t *testing.T) {
	input := "\nSome answer\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	questions := []string{"First question?", "Second question?"}

	clarifications, err := AskQuestions(questions, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clarifications) != 2 {
		t.Fatalf("len(clarifications) = %d, want 2", len(clarifications))
	}

	// Skipped question should have empty answer
	if clarifications[0].Answer != "" {
		t.Errorf("clarifications[0].Answer = %q, want empty string", clarifications[0].Answer)
	}
	if clarifications[1].Answer != "Some answer" {
		t.Errorf("clarifications[1].Answer = %q, want %q", clarifications[1].Answer, "Some answer")
	}
}

func TestAskQuestions_SingleQuestion(t *testing.T) {
	input := "My answer\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	clarifications, err := AskQuestions([]string{"Only question?"}, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clarifications) != 1 {
		t.Fatalf("len(clarifications) = %d, want 1", len(clarifications))
	}
	if clarifications[0].Question != "Only question?" {
		t.Errorf("Question = %q, want %q", clarifications[0].Question, "Only question?")
	}
	if clarifications[0].Answer != "My answer" {
		t.Errorf("Answer = %q, want %q", clarifications[0].Answer, "My answer")
	}
}

func TestAskQuestions_OutputFormat(t *testing.T) {
	input := "answer1\nanswer2\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	questions := []string{"First?", "Second?"}

	_, err := AskQuestions(questions, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := w.String()

	if !strings.Contains(output, "1. First?") {
		t.Errorf("output missing numbered question 1, got:\n%s", output)
	}
	if !strings.Contains(output, "2. Second?") {
		t.Errorf("output missing numbered question 2, got:\n%s", output)
	}
	if !strings.Contains(output, "> ") {
		t.Errorf("output missing prompt indicator '> ', got:\n%s", output)
	}
}

func TestAskQuestions_EOFMidway(t *testing.T) {
	// Only one line of input but two questions — EOF after first answer
	input := "partial answer\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	questions := []string{"First?", "Second?", "Third?"}

	clarifications, err := AskQuestions(questions, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clarifications) != 3 {
		t.Fatalf("len(clarifications) = %d, want 3", len(clarifications))
	}
	if clarifications[0].Answer != "partial answer" {
		t.Errorf("clarifications[0].Answer = %q, want %q", clarifications[0].Answer, "partial answer")
	}
	// Remaining should have empty answers due to EOF
	if clarifications[1].Answer != "" {
		t.Errorf("clarifications[1].Answer = %q, want empty", clarifications[1].Answer)
	}
	if clarifications[2].Answer != "" {
		t.Errorf("clarifications[2].Answer = %q, want empty", clarifications[2].Answer)
	}
}

// ---------------------------------------------------------------------------
// ParseFollowUpOutput tests
// ---------------------------------------------------------------------------

func TestParseFollowUpOutput_WithQuestions(t *testing.T) {
	output := `Some reasoning text...

` + "```json" + `
{
  "reasoning": "The user said 'whatever works' for the auth approach",
  "questions": ["Should token refresh happen client-side or server-side?", "What token expiry?"]
}
` + "```" + `

Done.`

	questions, err := ParseFollowUpOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("len(questions) = %d, want 2", len(questions))
	}
	if questions[0] != "Should token refresh happen client-side or server-side?" {
		t.Errorf("questions[0] = %q, want %q", questions[0], "Should token refresh happen client-side or server-side?")
	}
	if questions[1] != "What token expiry?" {
		t.Errorf("questions[1] = %q, want %q", questions[1], "What token expiry?")
	}
}

func TestParseFollowUpOutput_EmptyQuestions(t *testing.T) {
	output := "```json\n" + `{"reasoning": "All clear", "questions": []}` + "\n```"

	questions, err := ParseFollowUpOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 0 {
		t.Errorf("len(questions) = %d, want 0", len(questions))
	}
}

func TestParseFollowUpOutput_NoJSONBlock(t *testing.T) {
	output := "Just some text with no JSON block"

	_, err := ParseFollowUpOutput(output)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no JSON block") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "no JSON block")
	}
}

func TestParseFollowUpOutput_InvalidJSON(t *testing.T) {
	output := "```json\n{not valid json}\n```"

	_, err := ParseFollowUpOutput(output)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unmarshal")
	}
}

// ---------------------------------------------------------------------------
// RunClarificationLoop tests
// ---------------------------------------------------------------------------

func TestRunClarificationLoop_SimpleSkips(t *testing.T) {
	var stdout bytes.Buffer
	clarifications, usage, err := RunClarificationLoop(context.Background(), ClarifyOptions{
		Discovery: &DiscoveryResult{
			Complexity: ComplexitySimple,
			Questions:  []string{"Some question?"},
		},
		Complexity: ComplexitySimple,
		Stdin:      strings.NewReader(""),
		Stdout:     &stdout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clarifications != nil {
		t.Errorf("clarifications = %v, want nil", clarifications)
	}
	if !usage.IsZero() {
		t.Errorf("usage should be zero, got %+v", usage)
	}
}

func TestRunClarificationLoop_NoQuestions(t *testing.T) {
	var stdout bytes.Buffer
	clarifications, usage, err := RunClarificationLoop(context.Background(), ClarifyOptions{
		Discovery: &DiscoveryResult{
			Complexity: ComplexityMedium,
			Questions:  []string{},
		},
		Complexity: ComplexityMedium,
		Stdin:      strings.NewReader(""),
		Stdout:     &stdout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clarifications != nil {
		t.Errorf("clarifications = %v, want nil", clarifications)
	}
	if !usage.IsZero() {
		t.Errorf("usage should be zero, got %+v", usage)
	}
}

func TestRunClarificationLoop_SingleRound(t *testing.T) {
	// Provide answers for one round; follow-up agent will fail (no real agent)
	// which is non-fatal, so the loop should return after round 0.
	input := "Google and GitHub\nJWTs\n"
	var stdout bytes.Buffer

	clarifications, _, err := RunClarificationLoop(context.Background(), ClarifyOptions{
		Discovery: &DiscoveryResult{
			TaskSummary:  "Add OAuth",
			Complexity:   ComplexityMedium,
			WorkflowType: "feature",
			Questions:    []string{"Which providers?", "JWT or sessions?"},
		},
		Complexity:  ComplexityMedium,
		MaxRounds:   3,
		CommandName: "false", // will fail immediately — simulates agent failure
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clarifications) != 2 {
		t.Fatalf("len(clarifications) = %d, want 2", len(clarifications))
	}
	if clarifications[0].Answer != "Google and GitHub" {
		t.Errorf("clarifications[0].Answer = %q, want %q", clarifications[0].Answer, "Google and GitHub")
	}
	if clarifications[1].Answer != "JWTs" {
		t.Errorf("clarifications[1].Answer = %q, want %q", clarifications[1].Answer, "JWTs")
	}

	output := stdout.String()
	if !strings.Contains(output, "The following questions need your input") {
		t.Errorf("output missing initial prompt, got:\n%s", output)
	}
	if !strings.Contains(output, "Evaluating answers") {
		t.Errorf("output missing evaluation message, got:\n%s", output)
	}
}
