package block

import (
	"testing"

	"github.com/nwiley/arc/internal/resources"
)

func TestQABlockStructure(t *testing.T) {
	data, err := resources.BlockBytes("qa")
	if err != nil {
		t.Fatalf("loading qa block: %v", err)
	}
	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing qa block: %v", err)
	}

	if len(b.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(b.States))
	}
	if b.Entry != "qa" {
		t.Errorf("expected entry 'qa', got %q", b.Entry)
	}
	if len(b.Exits) != 1 || b.Exits[0] != "done" {
		t.Errorf("expected exits [done], got %v", b.Exits)
	}

	// Linear state: next is $done
	state := b.States[0]
	if next := state.Next[""]; next != "$done" {
		t.Errorf("expected linear next $done, got %q", next)
	}

	// max_turns param must have a positive default
	param, ok := b.Params["max_turns"]
	if !ok {
		t.Fatal("expected max_turns param")
	}
	if parseInt(param.Default) <= 0 {
		t.Errorf("max_turns default must be positive, got %q", param.Default)
	}
}

func TestQAReviewBlockStructure(t *testing.T) {
	data, err := resources.BlockBytes("qa-review")
	if err != nil {
		t.Fatalf("loading qa-review block: %v", err)
	}
	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing qa-review block: %v", err)
	}

	if len(b.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(b.States))
	}
	if b.Entry != "qa_review" {
		t.Errorf("expected entry 'qa_review', got %q", b.Entry)
	}

	// exits: [approved, gaps_found]
	exitSet := make(map[string]bool, len(b.Exits))
	for _, e := range b.Exits {
		exitSet[e] = true
	}
	if !exitSet["approved"] || !exitSet["gaps_found"] {
		t.Errorf("expected exits [approved, gaps_found], got %v", b.Exits)
	}
	if len(b.Exits) != 2 {
		t.Errorf("expected exactly 2 exits, got %d: %v", len(b.Exits), b.Exits)
	}

	state := b.States[0]
	if next := state.Next["approved"]; next != "$approved" {
		t.Errorf("approved should map to $approved, got %q", next)
	}
	if next := state.Next["gaps_found"]; next != "$gaps_found" {
		t.Errorf("gaps_found should map to $gaps_found, got %q", next)
	}

	// max_turns param must have a positive default
	param, ok := b.Params["max_turns"]
	if !ok {
		t.Fatal("expected max_turns param")
	}
	if parseInt(param.Default) <= 0 {
		t.Errorf("max_turns default must be positive, got %q", param.Default)
	}
}

func TestNewBlocksLoadClean(t *testing.T) {
	// Verify all new blocks in this phase load without error.
	newBlocks := []string{"qa", "qa-review"}
	for _, name := range newBlocks {
		t.Run(name, func(t *testing.T) {
			data, err := resources.BlockBytes(name)
			if err != nil {
				t.Fatalf("resources.BlockBytes(%q): %v", name, err)
			}
			if _, err := LoadBlock(data); err != nil {
				t.Fatalf("LoadBlock(%q): %v", name, err)
			}
		})
	}
}
