package prompt

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestExtractVerdictBasic(t *testing.T) {
	output := "Some analysis text...\n\n## Verdict\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictLastOccurrence(t *testing.T) {
	output := "## Verdict\n\ngaps_found\n\nMore analysis...\n\n## Verdict\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved, arc.VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q (should use last occurrence)", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictInsideCodeBlockIgnored(t *testing.T) {
	output := "```markdown\n## Verdict\ngaps_found\n```\n\n## Verdict\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved, arc.VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q (code block verdict should be ignored)", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictCodeBlockWithLanguageTag(t *testing.T) {
	output := "```go\n## Verdict\ngaps_found\n```\n\n## Verdict\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved, arc.VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictH3Header(t *testing.T) {
	output := "### Verdict\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictCaseInsensitiveHeader(t *testing.T) {
	output := "## VERDICT\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictNotFound(t *testing.T) {
	output := "No verdict section here"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err == nil {
		t.Fatal("expected error for missing verdict")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
	if !strings.Contains(err.Error(), "no verdict section found") {
		t.Fatalf("expected error containing 'no verdict section found', got: %v", err)
	}
}

func TestExtractVerdictEmptyOutput(t *testing.T) {
	v, err := ExtractVerdict("", []arc.Verdict{arc.VerdictApproved})
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
	if !strings.Contains(err.Error(), "no verdict section found") {
		t.Fatalf("expected error containing 'no verdict section found', got: %v", err)
	}
}

func TestExtractVerdictAllInCodeBlocks(t *testing.T) {
	output := "```\n## Verdict\napproved\n```\n\n```markdown\n## Verdict\ngaps_found\n```\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved, arc.VerdictGapsFound})
	if err == nil {
		t.Fatal("expected error when all verdicts are inside code blocks")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
	if !strings.Contains(err.Error(), "no verdict section found") {
		t.Fatalf("expected error containing 'no verdict section found', got: %v", err)
	}
}

func TestExtractVerdictIndentedBlockNotFence(t *testing.T) {
	// 4-space-indented backtick lines are NOT code fence toggles per the algorithm.
	output := "    ```\n    ## Verdict\n    gaps_found\n    ```\n\n## Verdict\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved, arc.VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictEmptyLinesBeforeValue(t *testing.T) {
	output := "## Verdict\n\n\n\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictWhitespaceOnVerdictLine(t *testing.T) {
	output := "## Verdict\n  approved  \n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictInvalidFormat(t *testing.T) {
	output := "## Verdict\n!!!invalid!!!\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err == nil {
		t.Fatal("expected error for invalid verdict format")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
}

func TestExtractVerdictH3Only(t *testing.T) {
	output := "Some text\n\n### Verdict\napproved\n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != arc.VerdictApproved {
		t.Fatalf("got %q, want %q", v, arc.VerdictApproved)
	}
}

func TestExtractVerdictWhitespaceOnlyAfterHeader(t *testing.T) {
	output := "## Verdict\n   \n   \n"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err == nil {
		t.Fatal("expected error when only whitespace after verdict header")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
}

func TestExtractVerdictEntireOutputCodeBlock(t *testing.T) {
	output := "```\n## Verdict\napproved\n```"
	v, err := ExtractVerdict(output, []arc.Verdict{arc.VerdictApproved})
	if err == nil {
		t.Fatal("expected error when verdict is inside code block")
	}
	if v != arc.VerdictUnknown {
		t.Fatalf("got %q, want %q", v, arc.VerdictUnknown)
	}
	if !strings.Contains(err.Error(), "no verdict section found") {
		t.Fatalf("expected error containing 'no verdict section found', got: %v", err)
	}
}
