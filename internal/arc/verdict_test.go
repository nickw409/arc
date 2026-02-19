package arc

import (
	"testing"
)

func TestParseVerdictExactMatch(t *testing.T) {
	v, err := ParseVerdict("approved", []Verdict{VerdictApproved, VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}

func TestParseVerdictCaseInsensitive(t *testing.T) {
	v, err := ParseVerdict("APPROVED", []Verdict{VerdictApproved, VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}

func TestParseVerdictWithWhitespace(t *testing.T) {
	v, err := ParseVerdict("  gaps_found  \n", []Verdict{VerdictApproved, VerdictGapsFound})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictGapsFound {
		t.Fatalf("got %q, want %q", v, VerdictGapsFound)
	}
}

func TestParseVerdictUnknown(t *testing.T) {
	v, err := ParseVerdict("maybe", []Verdict{VerdictApproved, VerdictGapsFound})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
	want := `verdict "maybe" not in valid set [approved gaps_found]`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseVerdictEmpty(t *testing.T) {
	v, err := ParseVerdict("", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
	if err.Error() != "empty verdict" {
		t.Fatalf("error = %q, want %q", err.Error(), "empty verdict")
	}
}

func TestParseVerdictMarkdownBold(t *testing.T) {
	v, err := ParseVerdict("**approved**", []Verdict{VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}

func TestParseVerdictMarkdownCode(t *testing.T) {
	v, err := ParseVerdict("`approved`", []Verdict{VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}

func TestParseVerdictExtraText(t *testing.T) {
	v, err := ParseVerdict("approved - looks good", []Verdict{VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}

func TestParseVerdictAllAdversaryVerdicts(t *testing.T) {
	tests := []struct {
		input string
		want  Verdict
	}{
		{"coverage_sufficient", VerdictCoverageSufficient},
		{"unambiguous", VerdictUnambiguous},
		{"consistent", VerdictConsistent},
		{"executable", VerdictExecutable},
		{"scope_appropriate", VerdictScopeAppropriate},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			v, err := ParseVerdict(tc.input, []Verdict{tc.want})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.want {
				t.Fatalf("got %q, want %q", v, tc.want)
			}
		})
	}
}

func TestVerdictIsValid(t *testing.T) {
	if !VerdictApproved.IsValid() {
		t.Fatal("VerdictApproved.IsValid() should be true")
	}
	if VerdictUnknown.IsValid() {
		t.Fatal("VerdictUnknown.IsValid() should be false")
	}
	if Verdict("").IsValid() {
		t.Fatal(`Verdict("").IsValid() should be false`)
	}
}

func TestParseVerdictWhitespaceOnly(t *testing.T) {
	v, err := ParseVerdict("   \n\t  ", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
	if err.Error() != "empty verdict" {
		t.Fatalf("error = %q, want %q", err.Error(), "empty verdict")
	}
}

func TestParseVerdictSpecialRegexChars(t *testing.T) {
	v, err := ParseVerdict("approved*", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error for invalid identifier, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
}

func TestParseVerdictNilValidSet(t *testing.T) {
	v, err := ParseVerdict("approved", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
	want := `verdict "approved" not in valid set []`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseVerdictUnicode(t *testing.T) {
	v, err := ParseVerdict("àpproved", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error for non-ASCII character, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
}

func TestParseVerdictMarkdownOnly(t *testing.T) {
	v, err := ParseVerdict("****", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
	if err.Error() != "empty verdict" {
		t.Fatalf("error = %q, want %q", err.Error(), "empty verdict")
	}
}

func TestParseVerdictBacktickOnly(t *testing.T) {
	v, err := ParseVerdict("`", []Verdict{VerdictApproved})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v != VerdictUnknown {
		t.Fatalf("got %q, want %q", v, VerdictUnknown)
	}
}

func TestParseVerdictMixedMarkdown(t *testing.T) {
	v, err := ParseVerdict("**`approved`**", []Verdict{VerdictApproved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictApproved {
		t.Fatalf("got %q, want %q", v, VerdictApproved)
	}
}
