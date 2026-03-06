package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
)

func TestParseStrategicDecision_ModifySpec(t *testing.T) {
	output := "MODIFY_SPEC\nThe spec is too broad. Narrow it to just the auth middleware.\n\n## New Spec\nAdd JWT validation middleware only."
	d := parseStrategicDecision(output)
	if d.Action != "modify_spec" {
		t.Errorf("action = %q, want modify_spec", d.Action)
	}
	if d.Reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
}

func TestParseStrategicDecision_AdjustGate(t *testing.T) {
	output := "ADJUST_GATE\nThe test_exists assertion for TestTokenExpiry is too strict.\n\n## Gate\nRemove test_exists:TestTokenExpiry"
	d := parseStrategicDecision(output)
	if d.Action != "adjust_gate" {
		t.Errorf("action = %q, want adjust_gate", d.Action)
	}
}

func TestParseStrategicDecision_SplitPhase(t *testing.T) {
	output := "SPLIT_PHASE\nThe phase covers both auth and rate limiting. Split into two."
	d := parseStrategicDecision(output)
	if d.Action != "split_phase" {
		t.Errorf("action = %q, want split_phase", d.Action)
	}
}

func TestParseStrategicDecision_GiveUp(t *testing.T) {
	output := "GIVE_UP\nThe required library is not compatible with Go 1.24."
	d := parseStrategicDecision(output)
	if d.Action != "give_up" {
		t.Errorf("action = %q, want give_up", d.Action)
	}
}

func TestParseStrategicDecision_EmptyOutput(t *testing.T) {
	d := parseStrategicDecision("")
	if d.Action != "give_up" {
		t.Errorf("action = %q, want give_up for empty output", d.Action)
	}
}

func TestParseStrategicDecision_InferAction(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"infer modify", "I think we should simplify the approach and modify the spec", "modify_spec"},
		{"infer split", "This phase is too large, we should split it into two", "split_phase"},
		{"infer gate", "The gate assertions are too strict, relax them", "adjust_gate"},
		{"infer give up", "This is impossible with the current constraints", "give_up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parseStrategicDecision(tt.output)
			if d.Action != tt.want {
				t.Errorf("action = %q, want %q", d.Action, tt.want)
			}
		})
	}
}

func TestApplyStrategicDecision_ModifySpec(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec: "Original spec",
	}
	decision := &StrategicDecision{
		Action:  "modify_spec",
		NewSpec: "Simplified spec",
	}
	if !applyStrategicDecision(decision, spec, nil) {
		t.Error("expected true for modify_spec with new spec")
	}
	if spec.Spec != "Simplified spec" {
		t.Errorf("spec = %q, want %q", spec.Spec, "Simplified spec")
	}
}

func TestApplyStrategicDecision_ModifySpecWithReasoning(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec: "Original spec",
	}
	decision := &StrategicDecision{
		Action:    "modify_spec",
		Reasoning: "Focus on the core logic only",
	}
	if !applyStrategicDecision(decision, spec, nil) {
		t.Error("expected true for modify_spec with reasoning")
	}
	if spec.Spec == "Original spec" {
		t.Error("spec should have been modified")
	}
}

func TestApplyStrategicDecision_AdjustGate(t *testing.T) {
	spec := &arc.PhaseSpec{
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "main.go", FileExists: "main.go"},
				{Type: "test_exists", Target: "TestFoo", TestExists: "TestFoo"},
			},
		},
	}
	gateResult := &arc.GateResult{
		Assertions: []arc.AssertionResult{
			{Description: "file_exists: main.go", Passed: true},
			{Description: "test_exists: TestFoo", Passed: false},
		},
	}
	decision := &StrategicDecision{
		Action: "adjust_gate",
	}
	if !applyStrategicDecision(decision, spec, gateResult) {
		t.Error("expected true for adjust_gate with failing assertion")
	}
	if len(spec.Gate.Assertions) != 1 {
		t.Errorf("expected 1 assertion after adjustment, got %d", len(spec.Gate.Assertions))
	}
	if spec.Gate.Assertions[0].Type != "file_exists" {
		t.Errorf("kept assertion type = %q, want file_exists", spec.Gate.Assertions[0].Type)
	}
}

func TestApplyStrategicDecision_AdjustGateNoop(t *testing.T) {
	spec := &arc.PhaseSpec{
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "main.go", FileExists: "main.go"},
			},
		},
	}
	gateResult := &arc.GateResult{
		Assertions: []arc.AssertionResult{
			{Description: "file_exists: main.go", Passed: true}, // all passing
		},
	}
	decision := &StrategicDecision{
		Action: "adjust_gate",
	}
	if applyStrategicDecision(decision, spec, gateResult) {
		t.Error("expected false when all assertions pass")
	}
}

func TestApplyStrategicDecision_GiveUp(t *testing.T) {
	spec := &arc.PhaseSpec{Spec: "test"}
	decision := &StrategicDecision{Action: "give_up"}
	if applyStrategicDecision(decision, spec, nil) {
		t.Error("expected false for give_up")
	}
}

func TestApplyStrategicDecision_SplitPhase(t *testing.T) {
	spec := &arc.PhaseSpec{Spec: "test"}
	decision := &StrategicDecision{Action: "split_phase"}
	if applyStrategicDecision(decision, spec, nil) {
		t.Error("expected false for split_phase (not handled inline)")
	}
}

func TestBuildStrategicPrompt_FallbackInline(t *testing.T) {
	spec := &arc.PhaseSpec{
		Name: "test-phase",
		Spec: "Implement feature X",
	}
	history := []AttemptRecord{
		{
			Attempt:           1,
			GateOutput:        "FAIL\n- [x] file_exists: main.go\n- [ ] test_exists: TestFoo",
			CheckpointsPassed: 1,
			CheckpointsTotal:  2,
			DiffSummary:       "1 file changed",
		},
		{
			Attempt:           2,
			GateOutput:        "FAIL\n- [x] file_exists: main.go\n- [ ] test_exists: TestFoo",
			CheckpointsPassed: 1,
			CheckpointsTotal:  2,
			DiffSummary:       "1 file changed",
		},
	}

	p, err := buildStrategicPrompt(spec, history)
	if err != nil {
		t.Fatalf("buildStrategicPrompt: %v", err)
	}

	// Check that key content is present
	checks := []string{
		"test-phase",
		"Implement feature X",
		"Attempt 1",
		"Attempt 2",
		"MODIFY_SPEC",
		"GIVE_UP",
	}
	for _, check := range checks {
		if !strings.Contains(p, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestRunStrategicIntervention_Integration(t *testing.T) {
	// Test with a mock adapter that returns a GIVE_UP decision
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second, Output: "GIVE_UP\nThe phase requirements conflict with existing code."},
		},
	}
	registerMockAdapter(t, "strategic-mock", mock)

	workDir := t.TempDir()
	spec := &arc.PhaseSpec{
		Name: "stuck-phase",
		Spec: "Do something impossible",
	}
	history := []AttemptRecord{
		{Attempt: 1, GateOutput: "FAIL", CheckpointsPassed: 0, CheckpointsTotal: 3},
		{Attempt: 2, GateOutput: "FAIL", CheckpointsPassed: 0, CheckpointsTotal: 3},
	}

	decision, err := RunStrategicIntervention(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "stuck-phase",
		ProjectDir: workDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "strategic-mock"}},
		Logger:     slog.Default(),
	}, spec, history)

	if err != nil {
		t.Fatalf("RunStrategicIntervention: %v", err)
	}
	if decision.Action != "give_up" {
		t.Errorf("action = %q, want give_up", decision.Action)
	}
}


func writePlanJSON(t *testing.T, planDir string, phases []string) {
	t.Helper()
	meta := arc.NewPlanMeta("test", "feature", phases)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0o644)
}

