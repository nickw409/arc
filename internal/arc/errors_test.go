package arc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPhaseErrorFormat(t *testing.T) {
	err := &PhaseError{
		Phase:   "core",
		State:   "impl",
		Kind:    ErrSubprocess,
		Message: "agent timed out",
		Cause:   context.DeadlineExceeded,
	}
	want := "[core/impl] subprocess: agent timed out: context deadline exceeded"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestPhaseErrorFormatNoCause(t *testing.T) {
	err := &PhaseError{
		Phase:   "core",
		State:   "qa",
		Kind:    ErrConstraint,
		Message: "max iterations exceeded",
	}
	want := "[core/qa] constraint: max iterations exceeded"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestPhaseErrorUnwrap(t *testing.T) {
	cause := fmt.Errorf("disk full")
	err := &PhaseError{Cause: cause}
	if errors.Unwrap(err) != cause {
		t.Fatal("Unwrap should return the cause")
	}
}

func TestPhaseErrorUnwrapNil(t *testing.T) {
	err := &PhaseError{}
	if errors.Unwrap(err) != nil {
		t.Fatal("Unwrap should return nil when no cause")
	}
}

func TestPhaseErrorKindValues(t *testing.T) {
	kinds := []struct {
		kind PhaseErrorKind
		val  int
	}{
		{ErrIteration, 0},
		{ErrConstraint, 1},
		{ErrEscalation, 2},
		{ErrIntervention, 3},
		{ErrSubprocess, 4},
		{ErrStateParse, 5},
		{ErrVerdictParse, 6},
		{ErrWorkflow, 7},
	}

	seen := make(map[int]bool)
	for _, tc := range kinds {
		if int(tc.kind) != tc.val {
			t.Fatalf("kind %d: got %d, want %d", tc.val, int(tc.kind), tc.val)
		}
		if seen[int(tc.kind)] {
			t.Fatalf("duplicate kind value: %d", int(tc.kind))
		}
		seen[int(tc.kind)] = true
	}

	if len(kinds) != 8 {
		t.Fatalf("expected 8 kinds, got %d", len(kinds))
	}
}

func TestPhaseErrorInvalidKind(t *testing.T) {
	// A PhaseErrorKind outside the valid range should not panic.
	err := &PhaseError{
		Phase:   "p",
		State:   "s",
		Kind:    PhaseErrorKind(999),
		Message: "something went wrong",
	}
	// Must not panic.
	got := err.Error()
	if got == "" {
		t.Fatal("Error() returned empty string for invalid kind")
	}
	if !strings.Contains(got, "unknown") {
		t.Fatalf("Error() for invalid kind should contain 'unknown', got: %q", got)
	}
}

func TestPhaseErrorFormatAllKinds(t *testing.T) {
	kindNames := []struct {
		kind PhaseErrorKind
		name string
	}{
		{ErrIteration, "iteration"},
		{ErrConstraint, "constraint"},
		{ErrEscalation, "escalation"},
		{ErrIntervention, "intervention"},
		{ErrSubprocess, "subprocess"},
		{ErrStateParse, "state_parse"},
		{ErrVerdictParse, "verdict_parse"},
		{ErrWorkflow, "workflow"},
	}

	for _, tc := range kindNames {
		t.Run(tc.name, func(t *testing.T) {
			err := &PhaseError{
				Phase:   "p",
				State:   "s",
				Kind:    tc.kind,
				Message: "msg",
			}
			got := err.Error()
			want := fmt.Sprintf("[p/s] %s: msg", tc.name)
			if got != want {
				t.Fatalf("Error() = %q, want %q", got, want)
			}
		})
	}
}
