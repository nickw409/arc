package block

import (
	"testing"
)

func makeImplBlock() *Block {
	return &Block{
		Name:  "impl",
		Entry: "impl",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:   "impl",
				Prompt: "prompts/adversarial/impl.md",
				Next:   map[string]string{"": "$done"},
			},
		},
	}
}

func makeAdversaryBlock() *Block {
	return &Block{
		Name:  "adversary",
		Entry: "adversary",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:     "adversary",
				Prompt:   "prompts/adversarial/adversary.md",
				Verdicts: []string{"bugs_found", "no_bugs_found"},
				Next: map[string]string{
					"bugs_found":    "adversary",
					"no_bugs_found": "$done",
				},
			},
		},
	}
}

func TestComposeSequential(t *testing.T) {
	blocks := []ResolvedBlock{
		{Name: "impl", Block: makeImplBlock()},
		{Name: "adversary", Block: makeAdversaryBlock()},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	// Entry should be "impl.impl"
	if wf.EntryState != "impl.impl" {
		t.Fatalf("expected entry 'impl.impl', got %q", wf.EntryState)
	}

	// Should have: impl.impl, adversary.adversary, complete, blocked
	if len(wf.States) != 4 {
		t.Fatalf("expected 4 states, got %d", len(wf.States))
	}

	// Verify state names
	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	for _, expected := range []string{"impl.impl", "adversary.adversary", "complete", "blocked"} {
		if !names[expected] {
			t.Fatalf("missing state %q", expected)
		}
	}

	// Verify impl.impl exits to adversary.adversary
	for _, s := range wf.States {
		if s.Name == "impl.impl" {
			next := s.Transition.Branches[""]
			if next != "adversary.adversary" {
				t.Fatalf("impl.impl should transition to adversary.adversary, got %q", next)
			}
		}
	}

	// Verify adversary.adversary no_bugs_found exits to complete, bugs_found self-loops
	for _, s := range wf.States {
		if s.Name == "adversary.adversary" {
			next := s.Transition.Branches["no_bugs_found"]
			if next != "complete" {
				t.Fatalf("adversary no_bugs_found should go to complete, got %q", next)
			}
			bugNext := s.Transition.Branches["bugs_found"]
			if bugNext != "adversary.adversary" {
				t.Fatalf("adversary bugs_found should self-loop to adversary.adversary, got %q", bugNext)
			}
		}
	}

	// Terminal states
	if len(wf.TerminalStates) != 2 {
		t.Fatalf("expected 2 terminal states, got %d", len(wf.TerminalStates))
	}
}

func TestComposeSequentialEmpty(t *testing.T) {
	_, err := ComposeSequential(nil)
	if err == nil {
		t.Fatal("expected error for empty blocks")
	}
}

func TestComposePipelineSequential(t *testing.T) {
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl"},
		{Block: "adversary"},
	}

	wf, groups, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	if len(groups) != 0 {
		t.Fatalf("expected no parallel groups for sequential pipeline, got %d", len(groups))
	}

	if wf.EntryState != "impl.impl" {
		t.Fatalf("expected entry 'impl.impl', got %q", wf.EntryState)
	}
}

func TestComposePipelineParallel(t *testing.T) {
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl"},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "security", Block: "adversary", Params: map[string]string{"focus": "security"}},
					{Name: "correctness", Block: "adversary", Params: map[string]string{"focus": "correctness"}},
				},
			},
		},
	}

	wf, groups, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 parallel group, got %d", len(groups))
	}

	pg := groups[0]
	if pg.Strategy != "all" {
		t.Fatalf("expected strategy 'all', got %q", pg.Strategy)
	}
	if len(pg.Blocks) != 2 {
		t.Fatalf("expected 2 parallel blocks, got %d", len(pg.Blocks))
	}
	if pg.Blocks[0].Name != "security" {
		t.Fatalf("expected first block 'security', got %q", pg.Blocks[0].Name)
	}

	// The workflow should have fork and join states
	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	if !names["_fork_0"] {
		t.Fatal("missing _fork_0 state")
	}
	if !names["_join_0"] {
		t.Fatal("missing _join_0 state")
	}
}

func TestComposePipelineMissingBlock(t *testing.T) {
	steps := []PipelineStep{
		{Block: "nonexistent"},
	}
	_, _, err := ComposePipeline(steps, map[string]*Block{})
	if err == nil {
		t.Fatal("expected error for missing block")
	}
}

func TestValidateComposition(t *testing.T) {
	blocks := []ResolvedBlock{
		{Name: "impl", Block: makeImplBlock()},
		{Name: "adversary", Block: makeAdversaryBlock()},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	errs := ValidateComposition(wf, blocks)
	if len(errs) > 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
}
