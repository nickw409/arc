package block

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
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

func TestBlockStateToConfig_MaxStateIterations(t *testing.T) {
	tests := []struct {
		name            string
		constraint      *ConstraintRaw
		wantNil         bool
		wantMax         int
		wantMaxState    int
		wantOnMax       string
	}{
		{
			name: "valid max_state_iterations with on_max_iterations",
			constraint: &ConstraintRaw{
				MaxStateIterations: "3",
				OnMaxIterations:    "approved",
			},
			wantNil:      false,
			wantMaxState: 3,
			wantOnMax:    "approved",
		},
		{
			name: "zero max_state_iterations not set",
			constraint: &ConstraintRaw{
				MaxStateIterations: "0",
				OnMaxIterations:    "approved",
			},
			wantNil:   false, // OnMaxIterations is set so constraint is created
			wantOnMax: "approved",
		},
		{
			name:       "nil constraint",
			constraint: nil,
			wantNil:    true,
		},
		{
			name:       "empty constraint fields",
			constraint: &ConstraintRaw{},
			wantNil:    true,
		},
		{
			name: "negative max_state_iterations not set",
			constraint: &ConstraintRaw{
				MaxStateIterations: "-1",
			},
			wantNil: true,
		},
		{
			name: "on_max_iterations copied correctly",
			constraint: &ConstraintRaw{
				MaxStateIterations: "5",
				OnMaxIterations:    "no_bugs_found",
			},
			wantNil:      false,
			wantMaxState: 5,
			wantOnMax:    "no_bugs_found",
		},
		{
			name: "both max_iterations and max_state_iterations",
			constraint: &ConstraintRaw{
				MaxIterations:      "10",
				MaxStateIterations: "3",
				OnMaxIterations:    "approved",
			},
			wantNil:      false,
			wantMax:      10,
			wantMaxState: 3,
			wantOnMax:    "approved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := BlockState{
				Name:        "qa_review",
				Prompt:      "prompts/feature/qa-review.md",
				Verdicts:    []string{"approved", "gaps_found"},
				Constraints: tt.constraint,
				Next:        map[string]string{"approved": "$approved", "gaps_found": "$gaps_found"},
			}

			sc := blockStateToConfig(bs, "test")

			if tt.wantNil {
				if sc.Constraints != nil {
					t.Fatalf("expected nil constraints, got %+v", sc.Constraints)
				}
				return
			}

			if sc.Constraints == nil {
				t.Fatal("expected non-nil constraints, got nil")
			}

			if sc.Constraints.MaxIterations != tt.wantMax {
				t.Errorf("MaxIterations = %d, want %d", sc.Constraints.MaxIterations, tt.wantMax)
			}
			if sc.Constraints.MaxStateIterations != tt.wantMaxState {
				t.Errorf("MaxStateIterations = %d, want %d", sc.Constraints.MaxStateIterations, tt.wantMaxState)
			}
			if sc.Constraints.OnMaxIterations != tt.wantOnMax {
				t.Errorf("OnMaxIterations = %q, want %q", sc.Constraints.OnMaxIterations, tt.wantOnMax)
			}
		})
	}
}

// makeJudgeBlock makes a branching block with exits: [pass, fail].
func makeJudgeBlock() *Block {
	return &Block{
		Name:  "judge",
		Entry: "judge",
		Exits: []string{"pass", "fail"},
		States: []BlockState{
			{
				Name:     "judge",
				Prompt:   "prompts/adversarial/adversary.md",
				Verdicts: []string{"pass", "fail"},
				Next: map[string]string{
					"pass": "$pass",
					"fail": "$fail",
				},
			},
		},
	}
}

func TestComposePipelineConditionalRouting(t *testing.T) {
	// pipeline: impl → judge (pass→complete, fail→fix) → fix
	blockDefs := map[string]*Block{
		"impl":  makeImplBlock(),
		"judge": makeJudgeBlock(),
		"fix":   makeImplBlock(), // reuse impl structure, different name
	}
	// Override fix block name so namespacing is distinct.
	fixBlock := *makeImplBlock()
	fixBlock.Name = "fix"
	blockDefs["fix"] = &fixBlock

	steps := []PipelineStep{
		{Block: "impl", Name: "impl"},
		{
			Block: "judge",
			Name:  "judge",
			Route: map[string]string{
				"pass": "complete", // exit early
				"fail": "fix",      // go to fix step
			},
		},
		{Block: "fix", Name: "fix"},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// Find judge.judge state and verify its transitions.
	var judgeState *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "judge.judge" {
			judgeState = &wf.States[i]
			break
		}
	}
	if judgeState == nil {
		t.Fatal("judge.judge state not found")
	}

	// pass → complete (terminal, from route)
	if next := judgeState.Transition.Branches["pass"]; next != "complete" {
		t.Errorf("pass should route to complete, got %q", next)
	}
	// fail → fix.impl (fix step's entry, from route)
	if next := judgeState.Transition.Branches["fail"]; next != "fix.impl" {
		t.Errorf("fail should route to fix.impl, got %q", next)
	}
}

func TestComposePipelineRoutingFallsThrough(t *testing.T) {
	// Exits not in the route map fall through to the next sequential step.
	blockDefs := map[string]*Block{
		"judge": makeJudgeBlock(),
		"fix":   makeImplBlock(),
	}
	fixBlock := *makeImplBlock()
	fixBlock.Name = "fix"
	blockDefs["fix"] = &fixBlock

	steps := []PipelineStep{
		{
			Block: "judge",
			Name:  "judge",
			Route: map[string]string{
				"pass": "complete", // only route pass; fail falls through
			},
		},
		{Block: "fix", Name: "fix"},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var judgeState *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "judge.judge" {
			judgeState = &wf.States[i]
			break
		}
	}
	if judgeState == nil {
		t.Fatal("judge.judge state not found")
	}

	// pass → complete (explicit route)
	if next := judgeState.Transition.Branches["pass"]; next != "complete" {
		t.Errorf("pass should route to complete, got %q", next)
	}
	// fail → fix.impl (default sequential fallthrough)
	if next := judgeState.Transition.Branches["fail"]; next != "fix.impl" {
		t.Errorf("fail should fall through to fix.impl, got %q", next)
	}
}

func TestComposePipelineNamedSteps(t *testing.T) {
	// Name field controls namespace prefix, not block type.
	blockDefs := map[string]*Block{
		"impl": makeImplBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl", Name: "write-code"},
		{Block: "impl", Name: "fix-bugs"},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	if wf.EntryState != "write-code.impl" {
		t.Errorf("entry = %q, want write-code.impl", wf.EntryState)
	}

	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	if !names["write-code.impl"] {
		t.Error("missing state write-code.impl")
	}
	if !names["fix-bugs.impl"] {
		t.Error("missing state fix-bugs.impl")
	}

	// write-code.impl should wire to fix-bugs.impl (sequential default).
	for _, s := range wf.States {
		if s.Name == "write-code.impl" {
			if next := s.Transition.Branches[""]; next != "fix-bugs.impl" {
				t.Errorf("write-code.impl → %q, want fix-bugs.impl", next)
			}
		}
	}
}

func TestComposePipelineNoRouteSameAsDefault(t *testing.T) {
	// Pipeline with no route field is identical to sequential behavior.
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl"},
		{Block: "adversary"},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// adversary bugs_found should still wire to adversary.adversary (self-loop via internal ref)
	for _, s := range wf.States {
		if s.Name == "adversary.adversary" {
			if next := s.Transition.Branches["bugs_found"]; next != "adversary.adversary" {
				t.Errorf("bugs_found → %q, want adversary.adversary", next)
			}
			if next := s.Transition.Branches["no_bugs_found"]; next != "complete" {
				t.Errorf("no_bugs_found → %q, want complete", next)
			}
		}
	}
}

func TestComposePipelineRunOnce(t *testing.T) {
	// Pipeline: impl → adversary (run_once, skip_exit: no_bugs_found)
	// Expect: adversary entry state has RunOnce=true and SkipVerdict="no_bugs_found".
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{Block: "impl", Name: "impl"},
		{
			Block:    "adversary",
			Name:     "check",
			RunOnce:  true,
			SkipExit: "no_bugs_found",
			Route: map[string]string{
				"bugs_found":    "impl",
				"no_bugs_found": "complete",
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// Find the adversary entry state (check.adversary).
	var adversaryState *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "check.adversary" {
			adversaryState = &wf.States[i]
			break
		}
	}
	if adversaryState == nil {
		t.Fatal("check.adversary state not found")
	}

	if !adversaryState.RunOnce {
		t.Error("check.adversary should have RunOnce=true")
	}
	if adversaryState.SkipVerdict != "no_bugs_found" {
		t.Errorf("check.adversary SkipVerdict = %q, want %q", adversaryState.SkipVerdict, "no_bugs_found")
	}

	// Non-run_once steps should not have RunOnce set.
	for _, s := range wf.States {
		if s.Name == "impl.impl" && s.RunOnce {
			t.Error("impl.impl should not have RunOnce=true")
		}
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
