package orchestrator

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestDeriveModeQAReview(t *testing.T) {
	got := deriveMode("qa_review")
	if got != "qa-review" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "qa_review", got, "qa-review")
	}
}

func TestDeriveModeImplReview(t *testing.T) {
	got := deriveMode("impl_review")
	if got != "impl-review" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "impl_review", got, "impl-review")
	}
}

func TestDeriveModeQA(t *testing.T) {
	got := deriveMode("qa")
	if got != "qa" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "qa", got, "qa")
	}
}

func TestDeriveModeImpl(t *testing.T) {
	got := deriveMode("impl")
	if got != "impl" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "impl", got, "impl")
	}
}

func TestDeriveModeReview(t *testing.T) {
	got := deriveMode("review")
	if got != "review" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "review", got, "review")
	}
}

func TestDeriveModeFix(t *testing.T) {
	// "fix" doesn't contain "review" and isn't "qa", so falls through to "impl"
	got := deriveMode("fix")
	if got != "impl" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "fix", got, "impl")
	}
}

func TestDeriveModeUnknownState(t *testing.T) {
	got := deriveMode("something_else")
	if got != "impl" {
		t.Fatalf("deriveMode(%q) = %q, want %q", "something_else", got, "impl")
	}
}

func TestStringOrDefaultWithValue(t *testing.T) {
	s := "hello"
	got := stringOrDefault(&s, "default")
	if got != "hello" {
		t.Fatalf("stringOrDefault(&%q, %q) = %q, want %q", s, "default", got, "hello")
	}
}

func TestStringOrDefaultNil(t *testing.T) {
	got := stringOrDefault(nil, "default")
	if got != "default" {
		t.Fatalf("stringOrDefault(nil, %q) = %q, want %q", "default", got, "default")
	}
}

func TestStringOrDefaultEmptyString(t *testing.T) {
	s := ""
	got := stringOrDefault(&s, "default")
	if got != "" {
		t.Fatalf("stringOrDefault(&%q, %q) = %q, want %q", s, "default", got, "")
	}
}

func TestIsComingFromQAReviewTrue(t *testing.T) {
	ps := &arc.PhaseState{
		VerdictsHistory: []arc.VerdictEntry{
			{State: "impl", Verdict: "approved"},
			{State: "qa_review", Verdict: "approved"},
		},
	}
	if !isComingFromQAReview(ps) {
		t.Fatal("expected isComingFromQAReview to return true for last verdict qa_review/approved")
	}
}

func TestIsComingFromQAReviewFalseWrongState(t *testing.T) {
	ps := &arc.PhaseState{
		VerdictsHistory: []arc.VerdictEntry{
			{State: "impl_review", Verdict: "approved"},
		},
	}
	if isComingFromQAReview(ps) {
		t.Fatal("expected isComingFromQAReview to return false for impl_review state")
	}
}

func TestIsComingFromQAReviewFalseWrongVerdict(t *testing.T) {
	ps := &arc.PhaseState{
		VerdictsHistory: []arc.VerdictEntry{
			{State: "qa_review", Verdict: "concerns"},
		},
	}
	if isComingFromQAReview(ps) {
		t.Fatal("expected isComingFromQAReview to return false for concerns verdict")
	}
}

func TestIsComingFromQAReviewEmptyHistory(t *testing.T) {
	ps := &arc.PhaseState{
		VerdictsHistory: []arc.VerdictEntry{},
	}
	if isComingFromQAReview(ps) {
		t.Fatal("expected isComingFromQAReview to return false for empty history")
	}
}

func TestIsComingFromQAReviewNilHistory(t *testing.T) {
	ps := &arc.PhaseState{}
	if isComingFromQAReview(ps) {
		t.Fatal("expected isComingFromQAReview to return false for nil history")
	}
}

func TestTruncateShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Fatalf("truncate(%q, 10) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncateExactLength(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncateLongString(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "hello world", got, "hello")
	}
}

func TestTruncateEmptyString(t *testing.T) {
	got := truncate("", 5)
	if got != "" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "", got, "")
	}
}
