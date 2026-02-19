package arc

import (
	"testing"
)

func TestIterationResultActionValues(t *testing.T) {
	actions := []struct {
		action ResultAction
		val    int
	}{
		{ActionContinue, 0},
		{ActionRetry, 1},
		{ActionEscalate, 2},
		{ActionIntervene, 3},
		{ActionAbort, 4},
	}

	seen := make(map[int]bool)
	for _, tc := range actions {
		if int(tc.action) != tc.val {
			t.Fatalf("action %d: got %d, want %d", tc.val, int(tc.action), tc.val)
		}
		if seen[int(tc.action)] {
			t.Fatalf("duplicate action value: %d", int(tc.action))
		}
		seen[int(tc.action)] = true
	}
}

func TestResultActionString(t *testing.T) {
	tests := []struct {
		action ResultAction
		want   string
	}{
		{ActionContinue, "continue"},
		{ActionRetry, "retry"},
		{ActionEscalate, "escalate"},
		{ActionIntervene, "intervene"},
		{ActionAbort, "abort"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.action.String()
			if got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResultActionStringOutOfRange(t *testing.T) {
	got := ResultAction(99).String()
	if got != "unknown" {
		t.Fatalf("String() for out-of-range = %q, want %q", got, "unknown")
	}
}
