package block

import (
	"testing"

	"github.com/nwiley/arc/internal/resources"
)

func TestAdversaryBlockLinearized(t *testing.T) {
	data, err := resources.BlockBytes("adversary")
	if err != nil {
		t.Fatalf("loading adversary block: %v", err)
	}
	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing adversary block: %v", err)
	}

	if len(b.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(b.States))
	}

	// Both verdicts should exit to $done (no self-loop)
	state := b.States[0]
	if next, ok := state.Next["bugs_found"]; !ok || next != "$done" {
		t.Errorf("bugs_found should map to $done, got %q (ok=%v)", next, ok)
	}
	if next, ok := state.Next["no_bugs_found"]; !ok || next != "$done" {
		t.Errorf("no_bugs_found should map to $done, got %q (ok=%v)", next, ok)
	}

	// No constraints block
	if state.Constraints != nil {
		t.Errorf("expected no constraints, got %+v", state.Constraints)
	}

	// max_rounds param must be removed
	if _, ok := b.Params["max_rounds"]; ok {
		t.Error("max_rounds param should be removed from adversary block")
	}

	// exits must be [done]
	if len(b.Exits) != 1 || b.Exits[0] != "done" {
		t.Errorf("expected exits [done], got %v", b.Exits)
	}

	// entry must be adversary
	if b.Entry != "adversary" {
		t.Errorf("expected entry 'adversary', got %q", b.Entry)
	}
}

func TestReviewBlockNamedExits(t *testing.T) {
	data, err := resources.BlockBytes("review")
	if err != nil {
		t.Fatalf("loading review block: %v", err)
	}
	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing review block: %v", err)
	}

	// exits must be [approved, concerns]
	exitSet := make(map[string]bool, len(b.Exits))
	for _, e := range b.Exits {
		exitSet[e] = true
	}
	if !exitSet["approved"] || !exitSet["concerns"] {
		t.Errorf("expected exits [approved, concerns], got %v", b.Exits)
	}
	if len(b.Exits) != 2 {
		t.Errorf("expected exactly 2 exits, got %d", len(b.Exits))
	}

	if len(b.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(b.States))
	}
	state := b.States[0]

	// approved → $approved, concerns → $concerns (no self-loop)
	if next := state.Next["approved"]; next != "$approved" {
		t.Errorf("approved should map to $approved, got %q", next)
	}
	if next := state.Next["concerns"]; next != "$concerns" {
		t.Errorf("concerns should map to $concerns, got %q", next)
	}

	// entry must be impl_review
	if b.Entry != "impl_review" {
		t.Errorf("expected entry 'impl_review', got %q", b.Entry)
	}
}
