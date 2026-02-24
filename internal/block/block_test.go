package block

import (
	"testing"
)

func TestLoadBlock(t *testing.T) {
	data := []byte(`
name: test-block
description: A test block
params:
  max_turns: {default: "15"}
  focus: {default: ""}
entry: start
exits: [done]
states:
  - name: start
    description: Start state
    prompt: prompts/test/start.md
    verdicts: [pass, fail]
    next:
      pass: $done
      fail: retry
  - name: retry
    description: Retry state
    prompt: prompts/test/retry.md
    next: start
`)

	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	if b.Name != "test-block" {
		t.Fatalf("expected name 'test-block', got %q", b.Name)
	}
	if b.Entry != "start" {
		t.Fatalf("expected entry 'start', got %q", b.Entry)
	}
	if len(b.Exits) != 1 || b.Exits[0] != "done" {
		t.Fatalf("expected exits [done], got %v", b.Exits)
	}
	if len(b.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(b.States))
	}
	if len(b.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(b.Params))
	}
	if b.Params["max_turns"].Default != "15" {
		t.Fatalf("expected max_turns default '15', got %q", b.Params["max_turns"].Default)
	}
}

func TestLoadBlockNoName(t *testing.T) {
	data := []byte(`entry: start`)
	_, err := LoadBlock(data)
	if err == nil {
		t.Fatal("expected error for block with no name")
	}
}

func TestLoadBlockNoEntry(t *testing.T) {
	data := []byte(`name: test`)
	_, err := LoadBlock(data)
	if err == nil {
		t.Fatal("expected error for block with no entry")
	}
}

func TestResolveParams(t *testing.T) {
	data := []byte(`
name: test
description: Test with ${focus} area
params:
  max_rounds: {default: "3"}
  focus: {default: "general"}
entry: start
exits: [done]
states:
  - name: start
    description: Focus on ${focus}
    prompt: prompts/test.md
    constraints:
      max_iterations: "${max_rounds}"
    next: $done
`)

	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	resolved, err := ResolveParams(b, map[string]string{"focus": "security", "max_rounds": "5"})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	if resolved.States[0].Description != "Focus on security" {
		t.Fatalf("expected description 'Focus on security', got %q", resolved.States[0].Description)
	}
	if resolved.States[0].Constraints == nil {
		t.Fatal("expected constraints to be set")
	}
	if resolved.States[0].Constraints.MaxIterations != "5" {
		t.Fatalf("expected max_iterations '5', got %q", resolved.States[0].Constraints.MaxIterations)
	}
}

func TestResolveParamsDefaults(t *testing.T) {
	data := []byte(`
name: test
description: test
params:
  max_rounds: {default: "3"}
entry: start
exits: [done]
states:
  - name: start
    prompt: prompts/test.md
    constraints:
      max_iterations: "${max_rounds}"
    next: $done
`)

	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	// Resolve with no overrides — should use default "3"
	resolved, err := ResolveParams(b, nil)
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	if resolved.States[0].Constraints.MaxIterations != "3" {
		t.Fatalf("expected max_iterations '3' from default, got %q", resolved.States[0].Constraints.MaxIterations)
	}
}
