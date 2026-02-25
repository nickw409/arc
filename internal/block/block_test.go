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

func TestResolveParamsPrompt(t *testing.T) {
	data := []byte(`
name: test
description: test
params:
  prompt: {default: "prompts/default.md"}
entry: start
exits: [done]
states:
  - name: start
    prompt: ${prompt}
    next: $done
`)

	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	// Default prompt
	resolved, err := ResolveParams(b, nil)
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}
	if resolved.States[0].Prompt != "prompts/default.md" {
		t.Fatalf("expected default prompt, got %q", resolved.States[0].Prompt)
	}

	// Override prompt
	resolved, err = ResolveParams(b, map[string]string{"prompt": "prompts/custom.md"})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}
	if resolved.States[0].Prompt != "prompts/custom.md" {
		t.Fatalf("expected overridden prompt, got %q", resolved.States[0].Prompt)
	}
}

func TestResolveParamsVerdictsAndExits(t *testing.T) {
	data := []byte(`
name: judge
description: Generic judge
params:
  verdict_a: {default: "approved"}
  verdict_b: {default: "rejected"}
entry: judge
exits: ["${verdict_a}", "${verdict_b}"]
states:
  - name: judge
    prompt: prompts/judge.md
    verdicts: ["${verdict_a}", "${verdict_b}"]
    next:
      "${verdict_a}": "$${verdict_a}"
      "${verdict_b}": "$${verdict_b}"
`)

	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	resolved, err := ResolveParams(b, map[string]string{
		"verdict_a": "approved",
		"verdict_b": "gaps_found",
	})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	// Exits substituted
	if len(resolved.Exits) != 2 || resolved.Exits[0] != "approved" || resolved.Exits[1] != "gaps_found" {
		t.Errorf("exits = %v, want [approved gaps_found]", resolved.Exits)
	}

	// Verdicts substituted
	state := resolved.States[0]
	if len(state.Verdicts) != 2 || state.Verdicts[0] != "approved" || state.Verdicts[1] != "gaps_found" {
		t.Errorf("verdicts = %v, want [approved gaps_found]", state.Verdicts)
	}

	// Next map keys and values substituted: "approved" → "$approved"
	if next := state.Next["approved"]; next != "$approved" {
		t.Errorf("next[approved] = %q, want $approved", next)
	}
	if next := state.Next["gaps_found"]; next != "$gaps_found" {
		t.Errorf("next[gaps_found] = %q, want $gaps_found", next)
	}

	// Old keys must be gone
	if _, ok := state.Next["${verdict_a}"]; ok {
		t.Error("unreplaced key ${verdict_a} still present in next map")
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
