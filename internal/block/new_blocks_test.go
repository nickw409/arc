package block

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
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

	// prompt param must exist with non-empty default
	promptParam, ok := b.Params["prompt"]
	if !ok {
		t.Fatal("expected prompt param")
	}
	if promptParam.Default == "" {
		t.Error("prompt param default must be non-empty")
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

	// prompt param must exist with non-empty default
	promptParam, ok := b.Params["prompt"]
	if !ok {
		t.Fatal("expected prompt param")
	}
	if promptParam.Default == "" {
		t.Error("prompt param default must be non-empty")
	}
}

func TestJudgeBlockResolvesVerdicts(t *testing.T) {
	data, err := resources.BlockBytes("judge")
	if err != nil {
		t.Fatalf("loading judge block: %v", err)
	}
	b, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing judge block: %v", err)
	}

	// Resolve with specific verdicts
	resolved, err := ResolveParams(b, map[string]string{
		"verdict_a": "approved",
		"verdict_b": "gaps_found",
		"prompt":    "prompts/feature/qa-review.md",
	})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	// Exits
	exitSet := make(map[string]bool)
	for _, e := range resolved.Exits {
		exitSet[e] = true
	}
	if !exitSet["approved"] || !exitSet["gaps_found"] || len(resolved.Exits) != 2 {
		t.Errorf("exits = %v, want [approved, gaps_found]", resolved.Exits)
	}

	state := resolved.States[0]

	// Verdicts
	if len(state.Verdicts) != 2 || state.Verdicts[0] != "approved" || state.Verdicts[1] != "gaps_found" {
		t.Errorf("verdicts = %v, want [approved gaps_found]", state.Verdicts)
	}

	// Next refs
	if next := state.Next["approved"]; next != "$approved" {
		t.Errorf("next[approved] = %q, want $approved", next)
	}
	if next := state.Next["gaps_found"]; next != "$gaps_found" {
		t.Errorf("next[gaps_found] = %q, want $gaps_found", next)
	}

	// Prompt overridden
	if state.Prompt != "prompts/feature/qa-review.md" {
		t.Errorf("prompt = %q, want prompts/feature/qa-review.md", state.Prompt)
	}

	// prompt param must exist
	if _, ok := b.Params["prompt"]; !ok {
		t.Error("expected prompt param")
	}
}

func TestJudgeBlockInPipeline(t *testing.T) {
	data, err := resources.BlockBytes("judge")
	if err != nil {
		t.Fatalf("loading judge block: %v", err)
	}
	judgeDef, err := LoadBlock(data)
	if err != nil {
		t.Fatalf("parsing judge block: %v", err)
	}

	implData, err := resources.BlockBytes("impl")
	if err != nil {
		t.Fatalf("loading impl block: %v", err)
	}
	implDef, err := LoadBlock(implData)
	if err != nil {
		t.Fatalf("parsing impl block: %v", err)
	}

	steps := []PipelineStep{
		{Block: "impl", Name: "qa", Params: map[string]string{"prompt": "prompts/feature/qa.md"}},
		{
			Block: "judge",
			Name:  "qa-check",
			Params: map[string]string{
				"verdict_a": "approved",
				"verdict_b": "gaps_found",
				"prompt":    "prompts/feature/qa-review.md",
			},
			Route: map[string]string{
				"approved":   "impl-step",
				"gaps_found": "qa",
			},
		},
		{Block: "impl", Name: "impl-step", Params: map[string]string{"prompt": "prompts/feature/impl.md"}},
	}

	blockDefs := map[string]*Block{
		"impl":  implDef,
		"judge": judgeDef,
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// Find qa-check.judge state and verify routing
	var judgeState *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "qa-check.judge" {
			judgeState = &wf.States[i]
			break
		}
	}
	if judgeState == nil {
		t.Fatal("qa-check.judge state not found")
	}

	// approved → impl-step.impl
	if next := judgeState.Transition.Branches["approved"]; next != "impl-step.impl" {
		t.Errorf("approved → %q, want impl-step.impl", next)
	}
	// gaps_found → qa.impl (loops back)
	if next := judgeState.Transition.Branches["gaps_found"]; next != "qa.impl" {
		t.Errorf("gaps_found → %q, want qa.impl", next)
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
