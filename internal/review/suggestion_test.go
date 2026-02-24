package review

import (
	"strings"
	"testing"
)

func TestParseSuggestionsBasic(t *testing.T) {
	output := `## Coverage Analysis

Some findings here.

## Suggestions

<<<ORIGINAL
func Foo() error {
>>>
<<<SUGGESTED
func Foo() (string, error) {
>>>

## Verdict
coverage_gaps`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Original != "func Foo() error {" {
		t.Fatalf("unexpected Original: %q", suggestions[0].Original)
	}
	if suggestions[0].Suggested != "func Foo() (string, error) {" {
		t.Fatalf("unexpected Suggested: %q", suggestions[0].Suggested)
	}
	if suggestions[0].Adversary != "coverage" {
		t.Fatalf("unexpected Adversary: %q", suggestions[0].Adversary)
	}
	if suggestions[0].Priority != 2 {
		t.Fatalf("expected priority 2 for coverage, got %d", suggestions[0].Priority)
	}
}

func TestParseSuggestionsMultiple(t *testing.T) {
	output := `## Suggestions

<<<ORIGINAL
line one
>>>
<<<SUGGESTED
line one fixed
>>>

<<<ORIGINAL
line two
>>>
<<<SUGGESTED
line two fixed
>>>

## Verdict
ambiguous`

	suggestions := ParseSuggestions("ambiguity", output)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Original != "line one" {
		t.Fatalf("suggestion 0 Original: %q", suggestions[0].Original)
	}
	if suggestions[1].Original != "line two" {
		t.Fatalf("suggestion 1 Original: %q", suggestions[1].Original)
	}
}

func TestParseSuggestionsMultiline(t *testing.T) {
	output := `## Suggestions

<<<ORIGINAL
func Foo() {
    return nil
}
>>>
<<<SUGGESTED
func Foo() error {
    return fmt.Errorf("not implemented")
}
>>>

## Verdict
coverage_gaps`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if !strings.Contains(suggestions[0].Original, "return nil") {
		t.Fatalf("expected multiline Original, got: %q", suggestions[0].Original)
	}
	if !strings.Contains(suggestions[0].Suggested, "not implemented") {
		t.Fatalf("expected multiline Suggested, got: %q", suggestions[0].Suggested)
	}
}

func TestParseSuggestionsNoSuggestions(t *testing.T) {
	output := `## Coverage Analysis

Everything looks good.

## Verdict
coverage_sufficient`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestParseSuggestionsEmptyOriginalSkipped(t *testing.T) {
	output := `## Suggestions

<<<ORIGINAL
>>>
<<<SUGGESTED
something new
>>>

## Verdict
coverage_gaps`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions (empty original skipped), got %d", len(suggestions))
	}
}

func TestParseSuggestionsIdenticalSkipped(t *testing.T) {
	output := `## Suggestions

<<<ORIGINAL
same text
>>>
<<<SUGGESTED
same text
>>>

## Verdict
coverage_gaps`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions (identical skipped), got %d", len(suggestions))
	}
}

func TestParseSuggestionsMissingSuggested(t *testing.T) {
	output := `## Suggestions

<<<ORIGINAL
some text
>>>

## Verdict
coverage_gaps`

	suggestions := ParseSuggestions("coverage", output)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions (missing SUGGESTED), got %d", len(suggestions))
	}
}

func TestMergeSuggestionsEmpty(t *testing.T) {
	merged := MergeSuggestions(nil)
	if merged != nil {
		t.Fatalf("expected nil, got %v", merged)
	}
}

func TestMergeSuggestionsNoConflict(t *testing.T) {
	suggestions := []Suggestion{
		{Adversary: "coverage", Priority: 2, Original: "func A()", Suggested: "func A() error"},
		{Adversary: "ambiguity", Priority: 3, Original: "// TODO", Suggested: "// Returns the user ID"},
	}

	merged := MergeSuggestions(suggestions)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged, got %d", len(merged))
	}
}

func TestMergeSuggestionsPriorityOrder(t *testing.T) {
	suggestions := []Suggestion{
		{Adversary: "ambiguity", Priority: 3, Original: "shared text", Suggested: "ambiguity fix"},
		{Adversary: "executability", Priority: 0, Original: "shared text", Suggested: "executability fix"},
	}

	merged := MergeSuggestions(suggestions)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged (conflict resolved), got %d", len(merged))
	}
	if merged[0].Adversary != "executability" {
		t.Fatalf("expected executability to win conflict, got %q", merged[0].Adversary)
	}
}

func TestMergeSuggestionsOverlapDetection(t *testing.T) {
	suggestions := []Suggestion{
		{Adversary: "consistency", Priority: 1, Original: "func Foo() error {\n    return nil\n}", Suggested: "func Foo() error {\n    return fmt.Errorf(\"x\")\n}"},
		{Adversary: "coverage", Priority: 2, Original: "return nil", Suggested: "return fmt.Errorf(\"covered\")"},
	}

	merged := MergeSuggestions(suggestions)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged (overlap detected), got %d", len(merged))
	}
	if merged[0].Adversary != "consistency" {
		t.Fatalf("expected consistency to win (higher priority), got %q", merged[0].Adversary)
	}
}

func TestApplySuggestionsBasic(t *testing.T) {
	planMD := "# Plan\n\nfunc Foo() {\n    return nil\n}\n\nSome other text.\n"
	suggestions := []Suggestion{
		{Original: "func Foo() {", Suggested: "func Foo() error {"},
	}

	result, applied := ApplySuggestions(planMD, suggestions)
	if applied != 1 {
		t.Fatalf("expected 1 applied, got %d", applied)
	}
	if !strings.Contains(result, "func Foo() error {") {
		t.Fatalf("expected suggestion to be applied, got: %s", result)
	}
	if strings.Contains(result, "func Foo() {") && !strings.Contains(result, "func Foo() error {") {
		t.Fatal("original text should be replaced")
	}
}

func TestApplySuggestionsNoMatch(t *testing.T) {
	planMD := "# Plan\n\nNothing matches here.\n"
	suggestions := []Suggestion{
		{Original: "text not in plan", Suggested: "replacement"},
	}

	result, applied := ApplySuggestions(planMD, suggestions)
	if applied != 0 {
		t.Fatalf("expected 0 applied, got %d", applied)
	}
	if result != planMD {
		t.Fatalf("plan should be unchanged when no match")
	}
}

func TestApplySuggestionsMultiple(t *testing.T) {
	planMD := "line A\nline B\nline C\n"
	suggestions := []Suggestion{
		{Original: "line A", Suggested: "line A fixed"},
		{Original: "line C", Suggested: "line C fixed"},
	}

	result, applied := ApplySuggestions(planMD, suggestions)
	if applied != 2 {
		t.Fatalf("expected 2 applied, got %d", applied)
	}
	if !strings.Contains(result, "line A fixed") {
		t.Fatal("expected line A to be fixed")
	}
	if !strings.Contains(result, "line C fixed") {
		t.Fatal("expected line C to be fixed")
	}
	if !strings.Contains(result, "line B") {
		t.Fatal("expected line B to be untouched")
	}
}

func TestAdversaryPriorities(t *testing.T) {
	// Verify the priority order is what we expect
	expected := []struct {
		name     string
		priority int
	}{
		{"executability", 0},
		{"consistency", 1},
		{"coverage", 2},
		{"ambiguity", 3},
		{"scope", 4},
	}

	for _, exp := range expected {
		got, ok := adversaryPriority[exp.name]
		if !ok {
			t.Fatalf("missing priority for %q", exp.name)
		}
		if got != exp.priority {
			t.Fatalf("priority for %q: got %d, want %d", exp.name, got, exp.priority)
		}
	}
}

func TestOverlaps(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"abc", "abc", true},
		{"abc def", "abc", true},
		{"abc", "abc def", true},
		{"abc", "xyz", false},
		{"hello world", "world", true},
		{"world", "hello world", true},
	}

	for _, tt := range tests {
		got := overlaps(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("overlaps(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
