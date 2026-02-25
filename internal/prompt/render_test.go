package prompt

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestRenderSimpleTemplate(t *testing.T) {
	result, err := RenderString("Phase: {{.Phase}}, Iteration: {{.Iteration}}", TemplateContext{
		Phase:     "qa",
		Iteration: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Phase: qa, Iteration: 3" {
		t.Fatalf("got %q, want %q", result, "Phase: qa, Iteration: 3")
	}
}

func TestRenderWithPlanMD(t *testing.T) {
	planMD := "# My Plan\n\nThis is a multi-line\nplan document.\n"
	result, err := RenderString("## Plan\n{{.PlanMD}}", TemplateContext{
		PlanMD: planMD,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, planMD) {
		t.Fatalf("expected output to contain PlanMD content, got %q", result)
	}
}

func TestRenderWithStateFields(t *testing.T) {
	result, err := RenderString("Tests: {{index .State \"tests_passing\"}}/{{index .State \"tests_total\"}}", TemplateContext{
		State: map[string]string{
			"tests_passing": "5",
			"tests_total":   "10",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Tests: 5/10" {
		t.Fatalf("got %q, want %q", result, "Tests: 5/10")
	}
}

func TestRenderMissingKeyErrors(t *testing.T) {
	_, err := RenderString("{{.NonexistentField}}", TemplateContext{})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "NonexistentField") {
		t.Fatalf("expected error to contain 'NonexistentField', got: %v", err)
	}
}

func TestRenderEmptyTemplate(t *testing.T) {
	result, err := RenderString("", TemplateContext{Phase: "qa"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Fatalf("got %q, want empty string", result)
	}
}

func TestValidateTemplateValid(t *testing.T) {
	err := ValidateTemplate("{{.Phase}} is {{.Iteration}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTemplateSyntaxError(t *testing.T) {
	err := ValidateTemplate("{{.Phase")
	if err == nil {
		t.Fatal("expected parse error for invalid template syntax")
	}
}

func TestValidateTemplateControlStructures(t *testing.T) {
	err := ValidateTemplate("{{if .Phase}}phase={{.Phase}}{{end}} {{range $k, $v := .State}}{{$k}}={{$v}} {{end}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStateToTemplateMap(t *testing.T) {
	state := &arc.PhaseState{
		TestsPassing: 5,
		TestsTotal:   10,
		Iteration:    arc.Iteration{Current: 3, Max: 25},
	}
	m := StateToTemplateMap(state)

	checks := map[string]string{
		"tests_passing":  "5",
		"tests_total":    "10",
		"iteration":      "3",
		"max_iterations": "25",
	}
	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("key %q: got %q, want %q", key, got, want)
		}
	}
}

func TestStateToTemplateMapAllKeys(t *testing.T) {
	state := &arc.PhaseState{
		Phase:              "test-phase",
		Plan:               "test-plan",
		WorkflowType:       "feature",
		PhaseStatus:        "implementing",
		CurrentState:       "impl",
		Iteration:          arc.Iteration{Current: 3, Max: 25},
		TestsPassing:       5,
		TestsTotal:         10,
		StuckIterations:    2,
		HangCount:          1,
		LastVerdict:        "approved",
		LastReviewedIter:   2,
		LastQAReviewedIter: 1,
		RollbackCount:      0,
		GlobalIterations:   15,
		LastCommit:         "abc123",
		ModelOverride:      "sonnet",
	}
	m := StateToTemplateMap(state)

	expectedKeys := []string{
		"phase", "plan", "workflow_type", "phase_status", "current_state",
		"iteration", "max_iterations", "tests_passing", "tests_total",
		"stuck_iterations", "hang_count", "last_verdict",
		"last_reviewed_iteration", "last_qa_reviewed_iteration",
		"rollback_count", "global_iterations", "last_commit", "model_override",
		"adversary_round",
	}

	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("missing expected key %q", key)
		}
	}

	// Ensure no extra keys
	for key := range m {
		found := false
		for _, ek := range expectedKeys {
			if key == ek {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected extra key %q", key)
		}
	}
}

func TestStateToTemplateMapExactCount(t *testing.T) {
	state := &arc.PhaseState{
		Phase:              "p",
		Plan:               "pl",
		WorkflowType:       "feature",
		PhaseStatus:        "implementing",
		CurrentState:       "impl",
		Iteration:          arc.Iteration{Current: 1, Max: 25},
		TestsPassing:       0,
		TestsTotal:         0,
		StuckIterations:    0,
		HangCount:          0,
		LastVerdict:        "",
		LastReviewedIter:   0,
		LastQAReviewedIter: 0,
		RollbackCount:      0,
		GlobalIterations:   0,
		LastCommit:         "",
		ModelOverride:      "",
	}
	m := StateToTemplateMap(state)
	if len(m) != 19 {
		t.Fatalf("got %d keys, want exactly 19", len(m))
	}
}

func TestStateToTemplateMapNilPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil state, got none")
		}
	}()
	StateToTemplateMap(nil)
}

func TestRenderFromResources(t *testing.T) {
	// Test the file-based Render path using an embedded prompt.
	result, err := Render("feature/qa.md", TemplateContext{
		Phase:        "test-phase",
		Plan:         "test-plan",
		Iteration:    1,
		PlanMD:       "# Test Plan",
		State:        map[string]string{"iteration": "1"},
		PlanFile:     ".plans/test-plan/plan.md",
		PhaseDir:     ".plans/test-plan/phases/test-phase",
		StateFile:    ".plans/test-plan/phases/test-phase/state.json",
		ScriptsDir:   ".arc/scripts",
		Mode:         "implement",
		DisputeCount: 0,
		DisputeList:  "(none)",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty rendered string")
	}
}

func TestRenderPartialIncludesInlined(t *testing.T) {
	// Verify that {{> common/test-commands.md}} is actually inlined, not stripped.
	result, err := Render("feature/qa.md", TemplateContext{
		Phase:        "test-phase",
		Plan:         "test-plan",
		Iteration:    1,
		PlanMD:       "# Test Plan",
		State:        map[string]string{"iteration": "1"},
		Params:       map[string]string{},
		PlanFile:     ".plans/test-plan/plan.md",
		PhaseDir:     ".plans/test-plan/phases/test-phase",
		StateFile:    ".plans/test-plan/phases/test-phase/state.json",
		ScriptsDir:   ".arc/scripts",
		Mode:         "implement",
		DisputeCount: 0,
		DisputeList:  "(none)",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// test-commands.md content should be present
	if !strings.Contains(result, "Do NOT run test commands directly") {
		t.Fatal("expected rendered output to contain inlined test-commands.md content")
	}
	// reasoning-format.md content should be present
	if !strings.Contains(result, "Analysis") {
		t.Fatal("expected rendered output to contain inlined reasoning-format.md content")
	}
}

func TestRenderImplPartialIncludesInlined(t *testing.T) {
	// Verify feature/impl.md inlines partials correctly in both modes.

	// First run (no last_verdict): test-commands.md and reasoning-format.md are inlined.
	firstRun, err := Render("feature/impl.md", TemplateContext{
		Phase:        "test-phase",
		Plan:         "test-plan",
		Iteration:    1,
		PlanMD:       "# Test Plan",
		State:        map[string]string{"tests_passing": "0", "tests_total": "5"},
		Params:       map[string]string{},
		PlanFile:     ".plans/test-plan/plan.md",
		PhaseDir:     ".plans/test-plan/phases/test-phase",
		StateFile:    ".plans/test-plan/phases/test-phase/state.json",
		ScriptsDir:   ".arc/scripts",
		Mode:         "implement",
		DisputeCount: 0,
		DisputeList:  "(none)",
	})
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}
	if !strings.Contains(firstRun, "Do NOT run test commands directly") {
		t.Fatal("first run: expected inlined test-commands.md content")
	}
	if !strings.Contains(firstRun, "Analysis") {
		t.Fatal("first run: expected inlined reasoning-format.md content")
	}

	// Bug-fix re-run (last_verdict=bugs_found): do-not-rules.md is inlined.
	fixRun, err := Render("feature/impl.md", TemplateContext{
		Phase:        "test-phase",
		Plan:         "test-plan",
		Iteration:    2,
		PlanMD:       "# Test Plan",
		State:        map[string]string{"tests_passing": "0", "tests_total": "5", "last_verdict": "bugs_found"},
		Params:       map[string]string{},
		PlanFile:     ".plans/test-plan/plan.md",
		PhaseDir:     ".plans/test-plan/phases/test-phase",
		StateFile:    ".plans/test-plan/phases/test-phase/state.json",
		ScriptsDir:   ".arc/scripts",
		Mode:         "implement",
		DisputeCount: 0,
		DisputeList:  "(none)",
	})
	if err != nil {
		t.Fatalf("fix run: unexpected error: %v", err)
	}
	if !strings.Contains(fixRun, "Do NOT modify test files") {
		t.Fatal("fix run: expected inlined do-not-rules.md content")
	}
	if !strings.Contains(fixRun, "Analysis") {
		t.Fatal("fix run: expected inlined reasoning-format.md content")
	}
}

func TestRenderWithParams(t *testing.T) {
	result, err := RenderString("Action: {{index .Params \"key\"}}", TemplateContext{
		Params: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Action: value" {
		t.Fatalf("got %q, want %q", result, "Action: value")
	}
}

func TestRenderStringNilMaps(t *testing.T) {
	result, err := RenderString("static text", TemplateContext{State: nil, Params: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "static text" {
		t.Fatalf("got %q, want %q", result, "static text")
	}
}

func TestRenderStringNilMapsWithMapReference(t *testing.T) {
	_, err := RenderString("value={{index .State \"key\"}}", TemplateContext{State: nil, Params: nil})
	if err == nil {
		t.Fatal("expected error when accessing key on nil map with missingkey=error")
	}
}

func TestRenderNewContextFields(t *testing.T) {
	tmpl := "plan={{.PlanFile}} dir={{.PhaseDir}} state={{.StateFile}} scripts={{.ScriptsDir}} mode={{.Mode}} disputes={{.DisputeCount}} list={{.DisputeList}}"
	ctx := TemplateContext{
		PlanFile:     "/plans/p/plan.md",
		PhaseDir:     "/plans/p/phases/qa",
		StateFile:    "/plans/p/phases/qa/state.json",
		ScriptsDir:   "/home/.arc/scripts",
		Mode:         "implement",
		DisputeCount: 2,
		DisputeList:  "- **test1**: reason1\n- **test2**: reason2",
	}
	result, err := RenderString(tmpl, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "plan=/plans/p/plan.md dir=/plans/p/phases/qa state=/plans/p/phases/qa/state.json scripts=/home/.arc/scripts mode=implement disputes=2 list=- **test1**: reason1\n- **test2**: reason2"
	if result != want {
		t.Fatalf("got %q, want %q", result, want)
	}
}

func TestRenderHandlebarsNewFields(t *testing.T) {
	tmpl := "plan={{plan_file}} dir={{phase_dir}} state={{state_file}} scripts={{scripts_dir}} mode={{mode}} disputes={{dispute_count}}"
	ctx := TemplateContext{
		PlanFile:     "/plans/p/plan.md",
		PhaseDir:     "/plans/p/phases/qa",
		StateFile:    "/plans/p/phases/qa/state.json",
		ScriptsDir:   "/home/.arc/scripts",
		Mode:         "implement",
		DisputeCount: 3,
	}
	result, err := RenderString(tmpl, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "plan=/plans/p/plan.md dir=/plans/p/phases/qa state=/plans/p/phases/qa/state.json scripts=/home/.arc/scripts mode=implement disputes=3"
	if result != want {
		t.Fatalf("got %q, want %q", result, want)
	}
}

func TestFormatDisputeList(t *testing.T) {
	tests := []struct {
		name     string
		disputes []arc.Dispute
		want     string
	}{
		{
			name:     "empty",
			disputes: nil,
			want:     "(none)",
		},
		{
			name:     "empty slice",
			disputes: []arc.Dispute{},
			want:     "(none)",
		},
		{
			name: "single dispute",
			disputes: []arc.Dispute{
				{TestName: "TestFoo", Reason: "wrong assertion"},
			},
			want: "- **TestFoo**: wrong assertion",
		},
		{
			name: "multiple disputes",
			disputes: []arc.Dispute{
				{TestName: "TestFoo", Reason: "wrong assertion"},
				{TestName: "TestBar", Reason: "missing edge case"},
			},
			want: "- **TestFoo**: wrong assertion\n- **TestBar**: missing edge case",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDisputeList(tt.disputes)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
