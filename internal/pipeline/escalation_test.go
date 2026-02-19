package pipeline

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func intPtr(n int) *int { return &n }

func TestCheckEscalationAtIterationMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 3

	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(3),
			Action:      "switch_model",
			Params:      map[string]string{"model": "sonnet"},
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action, got nil")
	}
	if result.Action != "switch_model" {
		t.Fatalf("got Action=%q, want %q", result.Action, "switch_model")
	}
	if result.Params["model"] != "sonnet" {
		t.Fatalf("got Params[model]=%q, want %q", result.Params["model"], "sonnet")
	}
}

func TestCheckEscalationAtIterationNoMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 2

	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(3),
			Action:      "switch_model",
			Params:      map[string]string{"model": "sonnet"},
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil, got %+v", result)
	}
}

func TestCheckEscalationAfterIterationMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 6
	// "after_iteration_5" NOT in ExecutedEscalations

	rules := []arc.EscalationRule{
		{
			AfterIteration: intPtr(5),
			Action:         "request_human",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action, got nil")
	}
	if result.Action != "request_human" {
		t.Fatalf("got Action=%q, want %q", result.Action, "request_human")
	}
}

func TestCheckEscalationAfterIterationAlreadyExecuted(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 6
	state.ExecutedEscalations = []string{"after_iteration_5"}

	rules := []arc.EscalationRule{
		{
			AfterIteration: intPtr(5),
			Action:         "request_human",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (already executed), got %+v", result)
	}
}

func TestCheckEscalationEveryNMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 6

	rules := []arc.EscalationRule{
		{
			EveryNIterations: intPtr(2),
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action (6 %% 2 == 0), got nil")
	}
	if result.Action != "run_tests" {
		t.Fatalf("got Action=%q, want %q", result.Action, "run_tests")
	}
}

func TestCheckEscalationEveryNNoMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	rules := []arc.EscalationRule{
		{
			EveryNIterations: intPtr(2),
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (5 %% 2 != 0), got %+v", result)
	}
}

func TestCheckEscalationAfterIterationExactThreshold(t *testing.T) {
	// "after" means Current > N, not Current >= N
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	rules := []arc.EscalationRule{
		{
			AfterIteration: intPtr(5),
			Action:         "request_human",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (5 is not > 5), got %+v", result)
	}
}

func TestCheckEscalationEveryOne(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 3

	rules := []arc.EscalationRule{
		{
			EveryNIterations: intPtr(1),
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action (every 1 iteration when Current > 0), got nil")
	}
}

func TestCheckEscalationIterationZeroEveryN(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 0

	rules := []arc.EscalationRule{
		{
			EveryNIterations: intPtr(2),
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (Current == 0, spec requires Current > 0), got %+v", result)
	}
}

func TestCheckEscalationMultipleCriteria(t *testing.T) {
	// Rule with both at_iteration: 5 and every_n_iterations: 3 — OR logic
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	rules := []arc.EscalationRule{
		{
			AtIteration:      intPtr(5),
			EveryNIterations: intPtr(3),
			Action:           "switch_model",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action (at_iteration matches), got nil")
	}
	if result.Action != "switch_model" {
		t.Fatalf("got Action=%q, want %q", result.Action, "switch_model")
	}
}

func TestCheckEscalationEveryNZero(t *testing.T) {
	// EveryNIterations == 0 should be guarded against (skip to avoid division by zero)
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 3

	rules := []arc.EscalationRule{
		{
			EveryNIterations: intPtr(0),
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (EveryNIterations==0 should be skipped), got %+v", result)
	}
}

func TestCheckEscalationNoRules(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	result := CheckEscalation(state, []arc.EscalationRule{})
	if result != nil {
		t.Fatalf("expected nil for empty rules, got %+v", result)
	}

	result = CheckEscalation(state, nil)
	if result != nil {
		t.Fatalf("expected nil for nil rules, got %+v", result)
	}
}

func TestCheckEscalationNilState(t *testing.T) {
	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(3),
			Action:      "switch_model",
		},
	}

	result := CheckEscalation(nil, rules)
	if result != nil {
		t.Fatalf("expected nil for nil state, got %+v", result)
	}
}

func TestCheckEscalationAllCriteriaNil(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	rules := []arc.EscalationRule{
		{
			AtIteration:      nil,
			AfterIteration:   nil,
			EveryNIterations: nil,
			Action:           "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (all criteria nil), got %+v", result)
	}
}

func TestCheckEscalationAtIterationZero(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 0

	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(0),
			Action:      "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action (0 == 0 matches), got nil")
	}
}

func TestCheckEscalationFirstNoMatchSecondMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(10),
			Action:      "switch_model",
			Params:      map[string]string{"model": "opus"},
		},
		{
			AtIteration: intPtr(5),
			Action:      "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result == nil {
		t.Fatal("expected escalation action from second rule, got nil")
	}
	if result.Action != "run_tests" {
		t.Fatalf("got Action=%q, want %q (from second rule)", result.Action, "run_tests")
	}
}

func TestCheckEscalationNegativeAtIteration(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 0

	rules := []arc.EscalationRule{
		{
			AtIteration: intPtr(-1),
			Action:      "run_tests",
		},
	}

	result := CheckEscalation(state, rules)
	if result != nil {
		t.Fatalf("expected nil (negative AtIteration), got %+v", result)
	}
}
