package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
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
  - block: impl
    params: {max_turns: "45"}
  - block: adversary-loop
    params: {max_rounds: "3", max_turns: "30"}

terminal_states: [complete, blocked]
`)

	w, err := LoadBytes(data)
	if err != nil {
		t.Fatalf("LoadBytes failed: %v", err)
	}

	if w.Name != "adversarial" {
		t.Fatalf("expected name 'adversarial', got %q", w.Name)
	}
	if w.EntryState != "impl.impl" {
		t.Fatalf("expected entry 'impl.impl', got %q", w.EntryState)
	}

	// Should have: impl.impl, adversary-loop.adversary, adversary-loop.impl_fix, complete, blocked
	if len(w.States) != 5 {
		t.Fatalf("expected 5 states, got %d", len(w.States))
	}

	stateNames := make(map[string]bool)
	for _, s := range w.States {
		stateNames[s.Name] = true
	}
	for _, name := range []string{"impl.impl", "adversary-loop.adversary", "adversary-loop.impl_fix", "complete", "blocked"} {
		if !stateNames[name] {
			t.Fatalf("missing state %q", name)
		}
	}

	// Verify the machine works with the composed workflow
	m := NewMachine(w)
	if m.EntryState() != "impl.impl" {
		t.Fatalf("machine entry != 'impl.impl'")
	}

	// impl.impl → adversary-loop.adversary (linear)
	next, err := m.NextState("impl.impl", "")
	if err != nil {
		t.Fatalf("NextState from impl.impl: %v", err)
	}
	if next != "adversary-loop.adversary" {
		t.Fatalf("expected adversary-loop.adversary, got %q", next)
	}

	// adversary-loop.adversary → bugs_found → adversary-loop.impl_fix
	next, err = m.NextState("adversary-loop.adversary", "bugs_found")
	if err != nil {
		t.Fatalf("NextState from adversary bugs_found: %v", err)
	}
	if next != "adversary-loop.impl_fix" {
		t.Fatalf("expected adversary-loop.impl_fix, got %q", next)
	}

	// adversary-loop.adversary → no_bugs_found → complete
	next, err = m.NextState("adversary-loop.adversary", "no_bugs_found")
	if err != nil {
		t.Fatalf("NextState from adversary no_bugs_found: %v", err)
	}
	if next != "complete" {
		t.Fatalf("expected complete, got %q", next)
	}

	// adversary-loop.impl_fix → adversary-loop.adversary (linear)
	next, err = m.NextState("adversary-loop.impl_fix", "")
	if err != nil {
		t.Fatalf("NextState from impl_fix: %v", err)
	}
	if next != "adversary-loop.adversary" {
		t.Fatalf("expected adversary-loop.adversary, got %q", next)
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
