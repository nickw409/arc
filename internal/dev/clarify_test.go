package dev

import (
	"bytes"
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
