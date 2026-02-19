package pipeline

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestCheckInterventionTriggered(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = 5

	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= 5", Message: "Phase stuck"},
	}

	result := CheckIntervention(state, triggers)
	if result != "Phase stuck" {
		t.Fatalf("got %q, want %q", result, "Phase stuck")
	}
}

func TestCheckInterventionNotTriggered(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = 3

	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= 5", Message: "Phase stuck"},
	}

	result := CheckIntervention(state, triggers)
	if result != "" {
		t.Fatalf("got %q, want empty string", result)
	}
}

func TestCheckInterventionAllOperators(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		state     func() *arc.PhaseState
		want      bool
	}{
		{
			name:      "eq_match",
			condition: "hang_count == 3",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.HangCount = 3
				return s
			},
			want: true,
		},
		{
			name:      "neq_no_match",
			condition: "hang_count != 0",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.HangCount = 0
				return s
			},
			want: false,
		},
		{
			name:      "gt_no_match_equal",
			condition: "iteration > 10",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.Iteration.Current = 10
				return s
			},
			want: false,
		},
		{
			name:      "gt_match",
			condition: "iteration > 10",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.Iteration.Current = 11
				return s
			},
			want: true,
		},
		{
			name:      "lt_match",
			condition: "tests_passing < 5",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.TestsPassing = 3
				return s
			},
			want: true,
		},
		{
			name:      "lte_match",
			condition: "rollback_count <= 2",
			state: func() *arc.PhaseState {
				s := arc.NewPhaseState("plan", "phase", "feature")
				s.RollbackCount = 2
				return s
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			triggers := []arc.InterventionTrigger{
				{Condition: tc.condition, Message: "triggered"},
			}
			result := CheckIntervention(tc.state(), triggers)
			if tc.want && result == "" {
				t.Fatalf("expected trigger for condition %q, got empty", tc.condition)
			}
			if !tc.want && result != "" {
				t.Fatalf("expected no trigger for condition %q, got %q", tc.condition, result)
			}
		})
	}
}

func TestCheckInterventionUnknownField(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")

	triggers := []arc.InterventionTrigger{
		{Condition: "nonexistent_field >= 5", Message: "should not trigger"},
	}

	result := CheckIntervention(state, triggers)
	if result != "" {
		t.Fatalf("expected empty (unknown field skipped), got %q", result)
	}
}

func TestCheckInterventionMultipleTriggers(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = 2
	state.HangCount = 5

	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= 10", Message: "a"},
		{Condition: "hang_count >= 3", Message: "b"},
	}

	result := CheckIntervention(state, triggers)
	if result != "b" {
		t.Fatalf("got %q, want %q", result, "b")
	}
}

func TestCheckInterventionNonIntegerValue(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = 5

	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= abc", Message: "should not trigger"},
	}

	result := CheckIntervention(state, triggers)
	if result != "" {
		t.Fatalf("expected empty (non-integer value skipped), got %q", result)
	}
}

func TestCheckInterventionMalformedCondition(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")

	triggers := []arc.InterventionTrigger{
		{Condition: "bad syntax", Message: "should not trigger"},
	}

	result := CheckIntervention(state, triggers)
	if result != "" {
		t.Fatalf("expected empty (malformed condition skipped), got %q", result)
	}
}

func TestCheckInterventionEmptyTriggers(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = 100

	result := CheckIntervention(state, []arc.InterventionTrigger{})
	if result != "" {
		t.Fatalf("expected empty for empty triggers, got %q", result)
	}
}

func TestCheckInterventionNilState(t *testing.T) {
	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= 5", Message: "should not trigger"},
	}

	result := CheckIntervention(nil, triggers)
	if result != "" {
		t.Fatalf("expected empty for nil state, got %q", result)
	}
}

func TestCheckInterventionNegativeValue(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.StuckIterations = -1

	triggers := []arc.InterventionTrigger{
		{Condition: "stuck_iterations >= 0", Message: "triggered"},
	}

	result := CheckIntervention(state, triggers)
	if result != "" {
		t.Fatalf("expected empty (-1 >= 0 is false), got %q", result)
	}
}
