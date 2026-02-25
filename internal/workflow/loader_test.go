package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/resources"
	"gopkg.in/yaml.v3"
)

func testdataPath(name string) string {
	// Walk up from internal/workflow to project root, then into testdata/workflows
	return filepath.Join("..", "..", "testdata", "workflows", name)
}

func TestLoadLinearWorkflow(t *testing.T) {
	w, err := Load(testdataPath("valid-feature.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// qa is a linear state: next: qa_review (string)
	var qa *arc.StateConfig
	for i := range w.States {
		if w.States[i].Name == "qa" {
			qa = &w.States[i]
			break
		}
	}
	if qa == nil {
		t.Fatal("state 'qa' not found")
	}

	want := map[arc.Verdict]string{"": "qa_review"}
	if len(qa.Transition.Branches) != len(want) {
		t.Fatalf("Branches len = %d, want %d", len(qa.Transition.Branches), len(want))
	}
	for k, v := range want {
		if qa.Transition.Branches[k] != v {
			t.Fatalf("Branches[%q] = %q, want %q", k, qa.Transition.Branches[k], v)
		}
	}
}

func TestLoadBranchingWorkflow(t *testing.T) {
	w, err := Load(testdataPath("valid-feature.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	var qaReview *arc.StateConfig
	for i := range w.States {
		if w.States[i].Name == "qa_review" {
			qaReview = &w.States[i]
			break
		}
	}
	if qaReview == nil {
		t.Fatal("state 'qa_review' not found")
	}

	want := map[arc.Verdict]string{
		arc.VerdictApproved:  "impl",
		arc.VerdictGapsFound: "qa",
	}
	if len(qaReview.Transition.Branches) != len(want) {
		t.Fatalf("Branches len = %d, want %d", len(qaReview.Transition.Branches), len(want))
	}
	for k, v := range want {
		if qaReview.Transition.Branches[k] != v {
			t.Fatalf("Branches[%q] = %q, want %q", k, qaReview.Transition.Branches[k], v)
		}
	}
}

func TestLoadWorkflowStatesCount(t *testing.T) {
	w, err := Load(testdataPath("valid-feature.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(w.States) != 6 {
		t.Fatalf("len(States) = %d, want 6", len(w.States))
	}
}

func TestLoadLinearOnlyWorkflow(t *testing.T) {
	w, err := Load(testdataPath("valid-linear-only.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// All non-terminal states should have linear transitions
	for _, s := range w.States {
		if s.Name == "done" {
			continue // terminal
		}
		if len(s.Verdicts) != 0 {
			t.Fatalf("state %q has verdicts %v, want none", s.Name, s.Verdicts)
		}
		if s.Transition.Branches == nil {
			t.Fatalf("state %q has nil branches, want linear transition", s.Name)
		}
		if _, ok := s.Transition.Branches[""]; !ok {
			t.Fatalf("state %q missing linear branch key \"\"", s.Name)
		}
	}
}

func TestTransitionUnmarshalString(t *testing.T) {
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: "impl",
		Tag:   "!!str",
	}

	var tr arc.Transition
	if err := tr.UnmarshalYAML(node); err != nil {
		t.Fatalf("UnmarshalYAML error: %v", err)
	}

	want := map[arc.Verdict]string{"": "impl"}
	if len(tr.Branches) != len(want) {
		t.Fatalf("Branches len = %d, want %d", len(tr.Branches), len(want))
	}
	for k, v := range want {
		if tr.Branches[k] != v {
			t.Fatalf("Branches[%q] = %q, want %q", k, tr.Branches[k], v)
		}
	}
}

func TestTransitionUnmarshalMap(t *testing.T) {
	// Create a mapping node: {approved: impl, gaps_found: qa}
	node := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "approved", Tag: "!!str"},
			{Kind: yaml.ScalarNode, Value: "impl", Tag: "!!str"},
			{Kind: yaml.ScalarNode, Value: "gaps_found", Tag: "!!str"},
			{Kind: yaml.ScalarNode, Value: "qa", Tag: "!!str"},
		},
	}

	var tr arc.Transition
	if err := tr.UnmarshalYAML(node); err != nil {
		t.Fatalf("UnmarshalYAML error: %v", err)
	}

	want := map[arc.Verdict]string{
		arc.VerdictApproved:  "impl",
		arc.VerdictGapsFound: "qa",
	}
	if len(tr.Branches) != len(want) {
		t.Fatalf("Branches len = %d, want %d", len(tr.Branches), len(want))
	}
	for k, v := range want {
		if tr.Branches[k] != v {
			t.Fatalf("Branches[%q] = %q, want %q", k, tr.Branches[k], v)
		}
	}
}

func TestTransitionUnmarshalInvalidSequence(t *testing.T) {
	node := &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "a", Tag: "!!str"},
			{Kind: yaml.ScalarNode, Value: "b", Tag: "!!str"},
		},
	}

	var tr arc.Transition
	err := tr.UnmarshalYAML(node)
	if err == nil {
		t.Fatal("expected error for sequence node, got nil")
	}
	if want := "must be a string or verdict map"; !containsString(err.Error(), want) {
		t.Fatalf("error = %q, want containing %q", err.Error(), want)
	}
}

func TestTransitionUnmarshalNull(t *testing.T) {
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: "",
		Tag:   "!!null",
	}

	var tr arc.Transition
	err := tr.UnmarshalYAML(node)
	// Should not panic. Either returns nil branches or error.
	if err != nil {
		// Acceptable: error is fine
		return
	}
	// Acceptable: nil branches for null
	if tr.Branches != nil {
		// If branches were set, they should be empty or nil
		return
	}
}

func TestLoadBytesValid(t *testing.T) {
	data, err := os.ReadFile(testdataPath("valid-feature.yaml"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	w, err := LoadBytes(data)
	if err != nil {
		t.Fatalf("LoadBytes failed: %v", err)
	}
	if len(w.States) != 6 {
		t.Fatalf("len(States) = %d, want 6", len(w.States))
	}
}

func TestLoadBytesEmpty(t *testing.T) {
	_, err := LoadBytes([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestLoadBytesMalformedYAML(t *testing.T) {
	_, err := LoadBytes([]byte("{invalid: yaml: [broken"))
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestLoadAdversarialPipelineWorkflow(t *testing.T) {
	data := []byte(`
name: adversarial
version: 1
description: Adversarial testing

pipeline:
  - block: act
    params: {max_turns: "45"}
  - block: adversary
    params: {max_turns: "30"}

terminal_states: [complete, blocked]
`)

	w, err := LoadBytes(data)
	if err != nil {
		t.Fatalf("LoadBytes failed: %v", err)
	}

	if w.Name != "adversarial" {
		t.Fatalf("expected name 'adversarial', got %q", w.Name)
	}
	if w.EntryState != "act.act" {
		t.Fatalf("expected entry 'act.act', got %q", w.EntryState)
	}

	// Should have: act.act, adversary.adversary, complete, blocked
	if len(w.States) != 4 {
		t.Fatalf("expected 4 states, got %d", len(w.States))
	}

	stateNames := make(map[string]bool)
	for _, s := range w.States {
		stateNames[s.Name] = true
	}
	for _, name := range []string{"act.act", "adversary.adversary", "complete", "blocked"} {
		if !stateNames[name] {
			t.Fatalf("missing state %q", name)
		}
	}

	// Verify the machine works with the composed workflow
	m := NewMachine(w)
	if m.EntryState() != "act.act" {
		t.Fatalf("machine entry != 'act.act'")
	}

	// act.act → adversary.adversary (linear)
	next, err := m.NextState("act.act", "")
	if err != nil {
		t.Fatalf("NextState from impl.impl: %v", err)
	}
	if next != "adversary.adversary" {
		t.Fatalf("expected adversary.adversary from act.act, got %q", next)
	}

	// adversary.adversary → bugs_found → complete (both exits wire to next step)
	next, err = m.NextState("adversary.adversary", "bugs_found")
	if err != nil {
		t.Fatalf("NextState from adversary bugs_found: %v", err)
	}
	if next != "complete" {
		t.Fatalf("expected complete (both exits wire to next step), got %q", next)
	}

	// adversary.adversary → no_bugs_found → complete
	next, err = m.NextState("adversary.adversary", "no_bugs_found")
	if err != nil {
		t.Fatalf("NextState from adversary no_bugs_found: %v", err)
	}
	if next != "complete" {
		t.Fatalf("expected complete, got %q", next)
	}
}

func TestConstraintsLoading(t *testing.T) {
	tests := []struct {
		workflow string
		state    string
		wantMax  int
		wantOn   string
	}{
		{"bugfix", "test_review", 3, "approved"},
		{"bugfix", "fix_review", 3, "approved"},
		{"refactor", "char_review", 3, "approved"},
		{"refactor", "verify", 3, "approved"},
		{"investigation", "review", 3, "approved"},
	}

	for _, tt := range tests {
		t.Run(tt.workflow+"_"+tt.state, func(t *testing.T) {
			data, err := resources.WorkflowBytes(tt.workflow)
			if err != nil {
				t.Fatalf("failed to load %s: %v", tt.workflow, err)
			}
			wf, err := LoadBytes(data)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", tt.workflow, err)
			}

			var found *arc.StateConfig
			for i := range wf.States {
				if wf.States[i].Name == tt.state {
					found = &wf.States[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("state %q not found in workflow %q", tt.state, tt.workflow)
			}
			if found.Constraints == nil {
				t.Fatalf("state %q in %q has no constraints", tt.state, tt.workflow)
			}
			if found.Constraints.MaxStateIterations != tt.wantMax {
				t.Errorf("MaxStateIterations = %d, want %d", found.Constraints.MaxStateIterations, tt.wantMax)
			}
			if found.Constraints.OnMaxIterations != tt.wantOn {
				t.Errorf("OnMaxIterations = %q, want %q", found.Constraints.OnMaxIterations, tt.wantOn)
			}
		})
	}
}

// containsString is a simple helper for checking substrings in error messages.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
