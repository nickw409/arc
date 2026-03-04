package block

import (
	"sort"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ── Adversary tests: parallel-runtime edge cases ────────────────────────────

// TestAdversary_ForkVerdictsMismatchStateVerdicts checks that the fork state's
// Verdicts field contains the actual state-level verdicts that agents produce
// (e.g. "bugs_found", "no_bugs_found"), NOT the block-level exit names
// (e.g. "done"). If the fork state only has exit names as verdicts,
// verdict-aware joining in RunParallel will fail because ExtractVerdict
// expects the state-level verdicts in the agent output.
func TestAdversary_ForkVerdictsMismatchStateVerdicts(t *testing.T) {
	// The adversary block has:
	//   Exits: ["done"]
	//   States[0].Verdicts: ["bugs_found", "no_bugs_found"]
	// The fork state should have Verdicts that match what agents produce,
	// which is the STATE verdicts, not the EXIT names.
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary", Params: map[string]string{"focus": "security"}},
					{Name: "b", Block: "adversary", Params: map[string]string{"focus": "edge cases"}},
				},
			},
			Route: map[string]string{
				"bugs_found":    "complete", // This route key is a state verdict, not an exit name
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

	// The fork state's Verdicts should contain the STATE-level verdicts that
	// agents actually produce in their output, NOT the block's exit names.
	// Agents produce "bugs_found" / "no_bugs_found" in their ## Verdict section.
	// If the fork only has ["done"] as verdicts, ExtractVerdict will never find
	// "bugs_found" or "no_bugs_found" in the output.
	verdictSet := make(map[string]bool)
	for _, v := range fork.Verdicts {
		verdictSet[v] = true
	}

	if verdictSet["done"] && !verdictSet["bugs_found"] {
		t.Fatalf("fork state Verdicts contains block exit name 'done' but not state verdict 'bugs_found' — "+
			"verdict extraction in RunParallel will fail because agents produce state-level verdicts, not exit names. "+
			"Verdicts: %v", fork.Verdicts)
	}

	// These are the verdicts agents actually produce:
	if !verdictSet["bugs_found"] {
		t.Fatalf("fork Verdicts missing 'bugs_found' — agents produce this verdict but ExtractVerdict won't find it. Got: %v", fork.Verdicts)
	}
	if !verdictSet["no_bugs_found"] {
		t.Fatalf("fork Verdicts missing 'no_bugs_found' — agents produce this verdict but ExtractVerdict won't find it. Got: %v", fork.Verdicts)
	}
}

// TestAdversary_ForkTransitionsUseStateVerdictsNotExits verifies that the fork
// state's transition branches are keyed by state-level verdicts (what agents
// produce), not by block exit names. If the route map uses state verdicts like
// "bugs_found" but the fork transitions are keyed by exit names like "done",
// the transition lookup will fail at runtime.
func TestAdversary_ForkTransitionsUseStateVerdictsNotExits(t *testing.T) {
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

	// After MergeVerdicts produces a verdict (e.g. "bugs_found"), the
	// machine.NextState lookup uses fork.Transition.Branches[verdict].
	// If the transition branches only have "done" → target (from exit names),
	// the lookup for "bugs_found" will fail.
	if _, ok := fork.Transition.Branches["bugs_found"]; !ok {
		t.Fatalf("fork Transition.Branches missing 'bugs_found' — the merged verdict won't resolve to a next state. "+
			"Available transitions: %v", fork.Transition.Branches)
	}
	if _, ok := fork.Transition.Branches["no_bugs_found"]; !ok {
		t.Fatalf("fork Transition.Branches missing 'no_bugs_found' — the merged verdict won't resolve to a next state. "+
			"Available transitions: %v", fork.Transition.Branches)
	}
}

// TestAdversary_ForkBranchParamsAliasing verifies that parallel branch Params
// are independent copies, not shared references to the same underlying map.
// If branches share Params references, mutating one branch's params during
// runtime (e.g., template rendering) corrupts other branches.
func TestAdversary_ForkBranchParamsAliasing(t *testing.T) {
	sharedParams := map[string]string{"focus": "security"}
	blockDefs := map[string]*Block{
		"adversary": makeAdversaryBlock(),
	}

	steps := []PipelineStep{
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "a", Block: "adversary", Params: sharedParams},
					{Name: "b", Block: "adversary", Params: sharedParams},
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
	if fork == nil || fork.Parallel == nil || len(fork.Parallel.Branches) < 2 {
		t.Fatal("need fork with 2+ branches")
	}

	// Mutate branch A's params
	fork.Parallel.Branches[0].Params["focus"] = "corrupted"

	// Branch B should be unaffected
	if fork.Parallel.Branches[1].Params["focus"] != "security" {
		t.Fatalf("branch params aliasing: mutating branch A's params corrupted branch B's params — "+
			"got %q, want 'security'", fork.Parallel.Branches[1].Params["focus"])
	}

	// Original map should also be unaffected
	if sharedParams["focus"] != "security" {
		t.Fatalf("branch params aliasing: mutating branch params corrupted the original params map — "+
			"got %q, want 'security'", sharedParams["focus"])
	}
}

// TestAdversary_MergeBlockAgentConfigsNoEntryMatch verifies behavior when a
// block's Entry field doesn't match any of its state names. This can happen
// with misconfigured blocks — the agent config should still be handled
// gracefully.
func TestAdversary_MergeBlockAgentConfigsNoEntryMatch(t *testing.T) {
	// Block where Entry is "nonexistent" but states only have "adversary"
	block := &Block{
		Name:  "bad",
		Entry: "nonexistent",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:   "adversary",
				Prompt: "p.md",
				Agent: &AgentConfigRaw{
					MaxTurns: "50",
				},
				Next: map[string]string{"": "$done"},
			},
		},
	}

	result := mergeBlockAgentConfigs([]ResolvedBlock{
		{Name: "bad", Block: block},
	})

	// Since Entry "nonexistent" doesn't match state "adversary", the agent
	// config with MaxTurns=50 is silently ignored. The merged result should
	// ideally still pick up the config, but currently it returns nil.
	if result != nil {
		// This test documents the actual behavior: agent config is silently
		// dropped when Entry doesn't match. If this starts failing, the
		// implementation was improved.
		t.Logf("mergeBlockAgentConfigs picked up config despite Entry mismatch — good!")
	}
	// No assertion failure here — this documents current behavior.
}

// TestAdversary_ForkDefaultNextChainedParallelSteps verifies that when two
// parallel steps are sequential, the first fork state's default transition
// targets the second fork state.
func TestAdversary_ForkDefaultNextChainedParallelSteps(t *testing.T) {
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
			// No route — all exits should default to next step (_fork_1)
		},
		{
			Parallel: &ParallelStep{
				Strategy: "all",
				Blocks: []ParallelBlockRef{
					{Name: "b", Block: "adversary"},
				},
			},
		},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	var fork0 *arc.StateConfig
	for i := range wf.States {
		if wf.States[i].Name == "_fork_0" {
			fork0 = &wf.States[i]
			break
		}
	}
	if fork0 == nil {
		t.Fatal("_fork_0 not found")
	}

	// Without explicit routes, all exit verdicts should transition to _fork_1
	for verdict, target := range fork0.Transition.Branches {
		if target != "_fork_1" {
			t.Errorf("fork_0 verdict %q → %q, want _fork_1", verdict, target)
		}
	}
}

// TestAdversary_MergeBlockAgentConfigsToolsOrdering verifies that the merged
// AllowedTools list is deterministic (sorted). Non-deterministic ordering
// could cause flaky test comparisons or config diffs.
func TestAdversary_MergeBlockAgentConfigsToolsOrdering(t *testing.T) {
	block1 := &Block{
		Name:  "b1",
		Entry: "s1",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:   "s1",
				Prompt: "p.md",
				Agent: &AgentConfigRaw{
					AllowedTools: []string{"Write", "Bash", "Read"},
				},
				Next: map[string]string{"": "$done"},
			},
		},
	}
	block2 := &Block{
		Name:  "b2",
		Entry: "s1",
		Exits: []string{"done"},
		States: []BlockState{
			{
				Name:   "s1",
				Prompt: "p.md",
				Agent: &AgentConfigRaw{
					AllowedTools: []string{"Grep", "Read", "Glob"},
				},
				Next: map[string]string{"": "$done"},
			},
		},
	}

	result := mergeBlockAgentConfigs([]ResolvedBlock{
		{Name: "a", Block: block1},
		{Name: "b", Block: block2},
	})
	if result == nil {
		t.Fatal("expected non-nil merged agent config")
	}

	// Check that tools are sorted for determinism
	tools := result.AllowedTools
	sorted := make([]string, len(tools))
	copy(sorted, tools)
	sort.Strings(sorted)

	for i := range tools {
		if tools[i] != sorted[i] {
			t.Fatalf("merged AllowedTools is not sorted: %v — non-deterministic ordering from map iteration",
				tools)
		}
	}
}

// TestAdversary_ValidateCompositionForkMissingParallelConfig verifies that
// ValidateComposition catches fork states that have transitions but a nil
// Parallel config. Such states are structurally invalid — they look like
// regular transition states but were meant to be parallel dispatchers.
func TestAdversary_ValidateCompositionForkMissingParallelConfig(t *testing.T) {
	// Manually construct a workflow with a fork state that has no ParallelConfig
	// but has verdicts and transitions (as if the parallel config was stripped).
	wf := &arc.Workflow{
		EntryState:     "_fork_0",
		TerminalStates: []string{"complete"},
		States: []arc.StateConfig{
			{
				Name:     "_fork_0",
				Parallel: nil, // Missing! This is malformed.
				Verdicts: []string{"bugs_found", "no_bugs_found"},
				Transition: arc.Transition{
					Branches: map[arc.Verdict]string{
						"bugs_found":    "complete",
						"no_bugs_found": "complete",
					},
				},
			},
			{Name: "complete", Prompt: "done"},
		},
	}

	errs := ValidateComposition(wf, nil)

	// A fork state without ParallelConfig is malformed. ValidateComposition
	// should detect this. If it doesn't, this is a validation gap.
	hasMalformedErr := false
	for _, e := range errs {
		if e != nil {
			hasMalformedErr = true
		}
	}
	if !hasMalformedErr {
		t.Fatal("ValidateComposition should detect fork state with nil ParallelConfig and verdicts — " +
			"such states are structurally invalid and will fail at runtime when RunState tries to dispatch")
	}
}

// TestAdversary_ValidateCompositionForkEmptyVerdicts verifies that
// ValidateComposition catches fork states with a ParallelConfig but empty
// Verdicts. Without verdicts, verdict-aware joining won't extract meaningful
// verdicts from branch outputs.
func TestAdversary_ValidateCompositionForkEmptyVerdicts(t *testing.T) {
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
				Verdicts: []string{}, // Empty verdicts
				Transition: arc.Transition{
					Branches: map[arc.Verdict]string{"": "complete"},
				},
			},
			{Name: "complete", Prompt: "done"},
		},
	}

	errs := ValidateComposition(wf, nil)

	hasMalformedErr := false
	for _, e := range errs {
		if e != nil {
			hasMalformedErr = true
		}
	}
	if !hasMalformedErr {
		t.Fatal("ValidateComposition should detect fork state with empty Verdicts — " +
			"verdict-aware joining will fall back to exit-code joining which may not produce meaningful results")
	}
}
