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
				Name:     "impl",
				Prompt:   "prompts/adversarial/impl.md",
				Verdicts: []string{"done"},
				Next:     map[string]string{"done": "$done"},
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
			next := s.Transition.Branches["done"]
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

	// The workflow should have _fork_0 but NOT _join_0 (join is handled by fork state)
	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	if !names["_fork_0"] {
		t.Fatal("missing _fork_0 state")
	}
	if names["_join_0"] {
		t.Fatal("_join_0 state should not exist (join handled by fork state)")
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

			sc := blockStateToConfig(bs, "test", nil)

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
			if next := s.Transition.Branches["done"]; next != "fix-bugs.impl" {
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

func TestBlockStateToConfigPassesParams(t *testing.T) {
	bs := BlockState{
		Name:   "adversary",
		Prompt: "prompts/adversarial/adversary.md",
	}
	params := map[string]string{"focus": "edge cases", "max_turns": "20"}
	sc := blockStateToConfig(bs, "test", params)
	if sc.Params == nil {
		t.Fatal("expected non-nil Params")
	}
	if sc.Params["focus"] != "edge cases" {
		t.Fatalf("expected focus = %q, got %q", "edge cases", sc.Params["focus"])
	}
	if sc.Params["max_turns"] != "20" {
		t.Fatalf("expected max_turns = %q, got %q", "20", sc.Params["max_turns"])
	}
}

func TestBlockStateToConfigNilParams(t *testing.T) {
	bs := BlockState{
		Name:   "adversary",
		Prompt: "prompts/adversarial/adversary.md",
	}
	sc := blockStateToConfig(bs, "test", nil)
	if sc.Params != nil {
		t.Fatalf("expected nil Params, got %v", sc.Params)
	}
}

func TestComposePipelineCarriesParams(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{
			Block:  "adversary",
			Name:   "adversary",
			Params: map[string]string{"focus": "security"},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// Find the adversary state
	var found *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "adversary.adversary" {
			found = &wf.States[i]
			break
		}
	}
	if found == nil {
		t.Fatal("adversary.adversary state not found")
	}
	if found.Params == nil {
		t.Fatal("expected non-nil Params on adversary state")
	}
	if found.Params["focus"] != "security" {
		t.Fatalf("expected Params[\"focus\"] == %q, got %q", "security", found.Params["focus"])
	}
}

func TestComposeSequentialCarriesParams(t *testing.T) {
	blocks := []ResolvedBlock{
		{Name: "impl", Block: makeImplBlock(), Params: map[string]string{"mode": "strict"}},
		{Name: "adversary", Block: makeAdversaryBlock()},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	// Find impl.impl state
	var found *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "impl.impl" {
			found = &wf.States[i]
			break
		}
	}
	if found == nil {
		t.Fatal("impl.impl state not found")
	}
	if found.Params == nil {
		t.Fatal("expected non-nil Params on impl state")
	}
	if found.Params["mode"] != "strict" {
		t.Fatalf("expected Params[\"mode\"] == %q, got %q", "strict", found.Params["mode"])
	}

	// adversary should have nil params (no params passed)
	for i := range wf.States {
		if wf.States[i].Name == "adversary.adversary" {
			if wf.States[i].Params != nil {
				t.Fatalf("expected nil Params on adversary state, got %v", wf.States[i].Params)
			}
		}
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

// ── Parallel fork state tests ─────────────────────────────────────────────────

func TestComposePipelineParallelForkHasParallelConfig(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
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

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil {
		t.Fatal("_fork_0 state not found")
	}
	if fork.Parallel == nil {
		t.Fatal("_fork_0 should have non-nil Parallel config")
	}
	if len(fork.Parallel.Branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(fork.Parallel.Branches))
	}
	if fork.Parallel.Strategy != "all" {
		t.Fatalf("expected strategy 'all', got %q", fork.Parallel.Strategy)
	}
}

func TestComposePipelineParallelForkHasVerdicts(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
					{Name: "b", Block: "adversary"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil {
		t.Fatal("_fork_0 not found")
	}

	// Fork verdicts come from the entry state's verdicts (what agents produce),
	// not block exit names.
	if len(fork.Verdicts) == 0 {
		t.Fatal("_fork_0 should have verdicts (from entry state)")
	}
	verdictSet := make(map[string]bool)
	for _, v := range fork.Verdicts {
		verdictSet[v] = true
	}
	if !verdictSet["bugs_found"] {
		t.Fatal("expected verdicts to contain 'bugs_found'")
	}
	if !verdictSet["no_bugs_found"] {
		t.Fatal("expected verdicts to contain 'no_bugs_found'")
	}
}

func TestComposePipelineParallelForkTransitions(t *testing.T) {
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{Block: "impl", Name: "impl"},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
					{Name: "b", Block: "adversary"},
				},
			},
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

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil {
		t.Fatal("_fork_0 not found")
	}
	if fork.Transition.Branches == nil {
		t.Fatal("_fork_0 should have transition branches")
	}
	target, ok := fork.Transition.Branches["bugs_found"]
	if !ok {
		t.Fatal("_fork_0 should have transition for 'bugs_found'")
	}
	if target != "impl.impl" {
		t.Fatalf("_fork_0 'bugs_found' transition target = %q, want %q", target, "impl.impl")
	}
	target, ok = fork.Transition.Branches["no_bugs_found"]
	if !ok {
		t.Fatal("_fork_0 should have transition for 'no_bugs_found'")
	}
	if target != "complete" {
		t.Fatalf("_fork_0 'no_bugs_found' transition target = %q, want %q", target, "complete")
	}
}

func TestComposePipelineParallelNoJoinState(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
					{Name: "b", Block: "adversary"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	for _, s := range wf.States {
		if s.Name == "_join_0" {
			t.Fatal("_join_0 should not exist")
		}
	}
}

func TestComposePipelineParallelBranchPrompts(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "security", Block: "adversary", Params: map[string]string{"focus": "security"}},
					{Name: "edges", Block: "adversary", Params: map[string]string{"focus": "edge cases"}},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil || fork.Parallel == nil {
		t.Fatal("_fork_0 not found or has no ParallelConfig")
	}

	for _, b := range fork.Parallel.Branches {
		if b.Prompt != "prompts/adversarial/adversary.md" {
			t.Fatalf("branch %q prompt = %q, want adversary prompt", b.Name, b.Prompt)
		}
	}
}

func TestComposePipelineParallelBranchParams(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "security", Block: "adversary", Params: map[string]string{"focus": "security"}},
					{Name: "edges", Block: "adversary", Params: map[string]string{"focus": "edge cases"}},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil || fork.Parallel == nil {
		t.Fatal("_fork_0 not found or has no ParallelConfig")
	}

	if fork.Parallel.Branches[0].Params["focus"] != "security" {
		t.Fatalf("branch 0 params[focus] = %q, want 'security'", fork.Parallel.Branches[0].Params["focus"])
	}
	if fork.Parallel.Branches[1].Params["focus"] != "edge cases" {
		t.Fatalf("branch 1 params[focus] = %q, want 'edge cases'", fork.Parallel.Branches[1].Params["focus"])
	}
}

func TestComposePipelineParallelAgentConfig(t *testing.T) {
	block1 := &Block{
		Name:  "adversary",
		Entry: "adversary",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:     "adversary",
				Prompt:   "prompts/adversarial/adversary.md",
				Verdicts: []string{"bugs_found", "no_bugs_found"},
				Next:     map[string]string{"bugs_found": "adversary", "no_bugs_found": "$done"},
				Agent: &AgentConfigRaw{
					MaxTurns:     "10",
					AllowedTools: []string{"Read", "Grep"},
				},
			},
		},
	}
	block2 := &Block{
		Name:  "adversary2",
		Entry: "adversary",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:     "adversary",
				Prompt:   "prompts/adversarial/adversary.md",
				Verdicts: []string{"bugs_found", "no_bugs_found"},
				Next:     map[string]string{"bugs_found": "adversary", "no_bugs_found": "$done"},
				Agent: &AgentConfigRaw{
					MaxTurns:     "15",
					AllowedTools: []string{"Read", "Write"},
				},
			},
		},
	}
	blockDefs := map[string]*Block{
		"adversary":  block1,
		"adversary2": block2,
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
					{Name: "b", Block: "adversary2"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil {
		t.Fatal("_fork_0 not found")
	}
	if fork.Agent == nil {
		t.Fatal("fork state should have merged agent config")
	}
	if fork.Agent.MaxTurns != 15 {
		t.Fatalf("expected max_turns=15 (max of 10 and 15), got %d", fork.Agent.MaxTurns)
	}
	// Union of tools: Read, Grep, Write
	toolSet := make(map[string]bool)
	for _, tool := range fork.Agent.AllowedTools {
		toolSet[tool] = true
	}
	for _, expected := range []string{"Read", "Grep", "Write"} {
		if !toolSet[expected] {
			t.Fatalf("expected allowed_tools to contain %q", expected)
		}
	}
}

func TestComposePipelineParallelSingleBlock(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "solo", Block: "adversary"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil || fork.Parallel == nil {
		t.Fatal("_fork_0 not found or has no ParallelConfig")
	}
	if len(fork.Parallel.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(fork.Parallel.Branches))
	}
}

func TestComposePipelineParallelEmptyBlocks(t *testing.T) {
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks:   []ParallelBlockRef{},
			},
		},
	}

	_, _, err := ComposePipeline(steps, map[string]*Block{})
	if err == nil {
		t.Fatal("expected error for empty parallel blocks")
	}
}

func TestComposePipelineMultipleParallelSteps(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
		"impl":      makeImplBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a1", Block: "adversary"},
					{Name: "a2", Block: "adversary"},
				},
			},
		},
		{Block: "impl", Name: "fix"},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "b1", Block: "adversary"},
					{Name: "b2", Block: "adversary"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	names := make(map[string]bool)
	for _, s := range wf.States {
		names[s.Name] = true
	}
	if !names["_fork_0"] {
		t.Fatal("missing _fork_0")
	}
	if !names["_fork_1"] {
		t.Fatal("missing _fork_1")
	}
	if names["_join_0"] || names["_join_1"] {
		t.Fatal("join states should not exist")
	}

	// Verify both forks have ParallelConfig
	for _, s := range wf.States {
		if s.Name == "_fork_0" || s.Name == "_fork_1" {
			if s.Parallel == nil {
				t.Fatalf("%s should have ParallelConfig", s.Name)
			}
		}
	}
}

func TestComposePipelineParallelMissingRouteTarget(t *testing.T) {
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
				},
			},
			Route: map[string]string{
				"bugs_found": "nonexistent_step",
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// The route target is a literal (not a known step), so it's wired as-is.
	// ValidateComposition should catch the invalid reference.
	errs := ValidateComposition(wf, nil)
	hasErr := false
	for _, e := range errs {
		if e != nil {
			hasErr = true
		}
	}
	if !hasErr {
		t.Fatal("expected ValidateComposition to report error for nonexistent target")
	}
}

func TestComposePipelineBackwardCompatibility(t *testing.T) {
	// Non-parallel pipeline should work exactly as before.
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
		t.Fatalf("expected no parallel groups, got %d", len(groups))
	}
	// No parallel states
	for _, s := range wf.States {
		if s.Parallel != nil {
			t.Fatalf("state %q should not have ParallelConfig", s.Name)
		}
	}
	if wf.EntryState != "impl.impl" {
		t.Fatalf("entry = %q, want impl.impl", wf.EntryState)
	}
}

func TestValidateCompositionParallelReachability(t *testing.T) {
	blockDefs := map[string]*Block{
		"impl":      makeImplBlock(),
		"adversary": makeAdversaryBlock(),
	}
	steps := []PipelineStep{
		{Block: "impl", Name: "impl"},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary"},
					{Name: "b", Block: "adversary"},
				},
			},
			Route: map[string]string{
				"done": "complete",
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	errs := ValidateComposition(wf, nil)
	if len(errs) > 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
}

func TestValidateCompositionParallelInvalidTransition(t *testing.T) {
	// Manually construct a fork state with transitions to non-existent targets.
	wf := &arc.Workflow{
		EntryState:     "_fork_0",
		TerminalStates: []string{"complete"},
		States: []arc.StateConfig{
			{
				Name: "_fork_0",
				Parallel: &arc.ParallelConfig{
					Branches: []arc.ParallelBranch{{Name: "a", Prompt: "test"}},
					Strategy: "all",
				},
				Verdicts:   []string{"done"},
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"done": "nonexistent_state"}},
			},
			{Name: "complete", Prompt: "done"},
		},
	}

	errs := ValidateComposition(wf, nil)
	hasRefErr := false
	for _, e := range errs {
		if e != nil {
			hasRefErr = true
		}
	}
	if !hasRefErr {
		t.Fatal("expected validation error for transition to non-existent state")
	}
}

func TestComposePipelineParallelActBlocks(t *testing.T) {
	// Parallel act blocks should produce a fork state with verdicts and valid transitions,
	// now that act blocks declare verdicts: [done].
	blockDefs := map[string]*Block{
		"impl": makeImplBlock(),
	}
	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "api", Block: "impl", Params: map[string]string{"focus": "API handlers"}},
					{Name: "core", Block: "impl", Params: map[string]string{"focus": "Core logic"}},
				},
			},
			Route: map[string]string{
				"done": "complete",
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

	var fork *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork = &wf.States[i]
			break
		}
	}
	if fork == nil {
		t.Fatal("_fork_0 not found")
	}

	// Fork should have verdict "done" from the act block entry state.
	if len(fork.Verdicts) != 1 || fork.Verdicts[0] != "done" {
		t.Fatalf("expected verdicts=[done], got %v", fork.Verdicts)
	}

	// Fork transition for "done" should route to complete.
	target, ok := fork.Transition.Branches["done"]
	if !ok {
		t.Fatal("_fork_0 should have transition for 'done'")
	}
	if target != "complete" {
		t.Fatalf("_fork_0 'done' → %q, want 'complete'", target)
	}

	// Verify parallel config has 2 branches with correct params.
	if fork.Parallel == nil {
		t.Fatal("_fork_0 should have ParallelConfig")
	}
	if len(fork.Parallel.Branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(fork.Parallel.Branches))
	}
	if fork.Parallel.Branches[0].Params["focus"] != "API handlers" {
		t.Fatalf("branch 0 focus = %q, want 'API handlers'", fork.Parallel.Branches[0].Params["focus"])
	}
	if fork.Parallel.Branches[1].Params["focus"] != "Core logic" {
		t.Fatalf("branch 1 focus = %q, want 'Core logic'", fork.Parallel.Branches[1].Params["focus"])
	}
}
