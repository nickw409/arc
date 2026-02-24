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

func makeAdversaryLoopBlock() *Block {
	return &Block{
		Name:  "adversary-loop",
		Entry: "adversary",
		Exits: []string{"converged"},
		States: []BlockState{
			{
				Name:     "adversary",
				Prompt:   "prompts/adversarial/adversary.md",
				Verdicts: []string{"bugs_found", "no_bugs_found"},
				Next: map[string]string{
					"bugs_found":    "impl_fix",
					"no_bugs_found": "$converged",
				},
			},
			{
				Name:   "impl_fix",
				Prompt: "prompts/adversarial/impl-fix.md",
				Next:   map[string]string{"": "adversary"},
			},
		},
	}
}

func TestComposeSequential(t *testing.T) {
	blocks := []ResolvedBlock{
		{Name: "impl", Block: makeImplBlock()},
		{Name: "adversary-loop", Block: makeAdversaryLoopBlock()},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	// Entry should be "impl.impl"
	if wf.EntryState != "impl.impl" {
		t.Fatalf("expected entry 'impl.impl', got %q", wf.EntryState)
	}

	// Should have: impl.impl, adversary-loop.adversary, adversary-loop.impl_fix, complete, blocked
	if len(wf.States) != 5 {
		t.Fatalf("expected 5 states, got %d", len(wf.States))
	}

	// Verify state names
	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	for _, expected := range []string{"impl.impl", "adversary-loop.adversary", "adversary-loop.impl_fix", "complete", "blocked"} {
		if !names[expected] {
			t.Fatalf("missing state %q", expected)
		}
	}

	// Verify impl.impl exits to adversary-loop.adversary
	for _, s := range wf.States {
		if s.Name == "impl.impl" {
			next := s.Transition.Branches[""]
			if next != "adversary-loop.adversary" {
				t.Fatalf("impl.impl should transition to adversary-loop.adversary, got %q", next)
			}
		}
	}

	// Verify adversary-loop.adversary no_bugs_found exits to complete
	for _, s := range wf.States {
		if s.Name == "adversary-loop.adversary" {
			next := s.Transition.Branches["no_bugs_found"]
			if next != "complete" {
				t.Fatalf("adversary no_bugs_found should go to complete, got %q", next)
			}
			bugNext := s.Transition.Branches["bugs_found"]
			if bugNext != "adversary-loop.impl_fix" {
				t.Fatalf("adversary bugs_found should go to adversary-loop.impl_fix, got %q", bugNext)
			}
		}
	}

	// Verify adversary-loop.impl_fix loops back to adversary-loop.adversary
	for _, s := range wf.States {
		if s.Name == "adversary-loop.impl_fix" {
			next := s.Transition.Branches[""]
			if next != "adversary-loop.adversary" {
				t.Fatalf("impl_fix should loop to adversary-loop.adversary, got %q", next)
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
		"impl":           makeImplBlock(),
		"adversary-loop": makeAdversaryLoopBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl"},
		{Block: "adversary-loop"},
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
		"impl":           makeImplBlock(),
		"adversary-loop": makeAdversaryLoopBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl"},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "security", Block: "adversary-loop", Params: map[string]string{"focus": "security"}},
					{Name: "correctness", Block: "adversary-loop", Params: map[string]string{"focus": "correctness"}},
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
		{Name: "adversary-loop", Block: makeAdversaryLoopBlock()},
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
