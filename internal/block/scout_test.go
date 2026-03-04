package block_test

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/block"
	"github.com/nwiley/arc/internal/resources"
)

func TestLoadScoutBlock(t *testing.T) {
	data, err := resources.BlockBytes("scout")
	if err != nil {
		t.Fatalf("BlockBytes(scout) failed: %v", err)
	}

	b, err := block.LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	if b.Name != "scout" {
		t.Errorf("name = %q, want %q", b.Name, "scout")
	}
	if b.Entry != "scout" {
		t.Errorf("entry = %q, want %q", b.Entry, "scout")
	}
	if len(b.Exits) != 1 || b.Exits[0] != "done" {
		t.Errorf("exits = %v, want [done]", b.Exits)
	}
	if len(b.States) != 1 {
		t.Errorf("states count = %d, want 1", len(b.States))
	}
	if b.States[0].Name != "scout" {
		t.Errorf("state name = %q, want %q", b.States[0].Name, "scout")
	}
}

func TestScoutBlockAllowedTools(t *testing.T) {
	data, err := resources.BlockBytes("scout")
	if err != nil {
		t.Fatalf("BlockBytes(scout) failed: %v", err)
	}

	b, err := block.LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	state := b.States[0]
	if state.Agent == nil {
		t.Fatal("state.Agent is nil")
	}

	want := []string{"Read", "Grep", "Glob"}
	if len(state.Agent.AllowedTools) != len(want) {
		t.Fatalf("allowed_tools = %v, want %v", state.Agent.AllowedTools, want)
	}
	for i, tool := range want {
		if state.Agent.AllowedTools[i] != tool {
			t.Errorf("allowed_tools[%d] = %q, want %q", i, state.Agent.AllowedTools[i], tool)
		}
	}

	// Verify no Write or Bash
	for _, tool := range state.Agent.AllowedTools {
		if tool == "Write" || tool == "Bash" {
			t.Errorf("scout must be read-only, but allowed_tools contains %q", tool)
		}
	}
}

func TestScoutBlockDefaults(t *testing.T) {
	data, err := resources.BlockBytes("scout")
	if err != nil {
		t.Fatalf("BlockBytes(scout) failed: %v", err)
	}

	b, err := block.LoadBlock(data)
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	resolved, err := block.ResolveParams(b, nil)
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	state := resolved.States[0]
	if state.Agent == nil {
		t.Fatal("state.Agent is nil")
	}

	// MaxTurns default = "10", Timeout default = "300"
	if state.Agent.MaxTurns != "10" {
		t.Errorf("max_turns = %q, want %q", state.Agent.MaxTurns, "10")
	}
	if state.Agent.Timeout != "300" {
		t.Errorf("timeout = %q, want %q", state.Agent.Timeout, "300")
	}
}

func TestScoutBlockInResolver(t *testing.T) {
	resolver := resources.NewResolver("", "")
	data, err := resolver.BlockBytes("scout")
	if err != nil {
		t.Fatalf("resolver.BlockBytes(scout) failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("resolver.BlockBytes(scout) returned empty data")
	}
}

func TestScoutBlockInListBlocks(t *testing.T) {
	resolver := resources.NewResolver("", "")
	blocks := resolver.ListBlocks()

	found := false
	for _, name := range blocks {
		if name == "scout" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListBlocks() = %v, does not contain 'scout'", blocks)
	}
}

func TestScoutPromptExists(t *testing.T) {
	data, err := resources.PromptBytes("blocks/scout.md")
	if err != nil {
		t.Fatalf("PromptBytes(blocks/scout.md) failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PromptBytes(blocks/scout.md) returned empty data")
	}

	content := string(data)
	if !strings.Contains(content, "Edge Cases") {
		t.Error("prompt does not contain 'Edge Cases'")
	}
}
