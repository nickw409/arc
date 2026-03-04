package block

import (
	"testing"
)

// TestAdversary_MultiStateBlockParamsAliasing verifies that states within the
// same block get independent copies of the params map. If blockStateToConfig
// assigns params by reference (sc.Params = params), all states from the same
// block share the same map — mutating one state's params corrupts the others.
func TestAdversary_MultiStateBlockParamsAliasing(t *testing.T) {
	// Block with two states
	block := &Block{
		Name:  "multi",
		Entry: "first",
		Exits: []string{"done"},
		States: []BlockState{
			{Name: "first", Prompt: "p1", Next: map[string]string{"": "second"}},
			{Name: "second", Prompt: "p2", Next: map[string]string{"": "$done"}},
		},
	}

	params := map[string]string{"focus": "security"}
	blocks := []ResolvedBlock{
		{Name: "multi", Block: block, Params: params},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	// Find both states from the composed workflow
	var firstState, secondState *stateRef
	for i := range wf.States {
		switch wf.States[i].Name {
		case "multi.first":
			firstState = &stateRef{idx: i}
		case "multi.second":
			secondState = &stateRef{idx: i}
		}
	}

	if firstState == nil || secondState == nil {
		t.Fatal("expected both multi.first and multi.second states to exist")
	}

	// Both states should have params
	if wf.States[firstState.idx].Params == nil {
		t.Fatal("multi.first should have non-nil Params")
	}
	if wf.States[secondState.idx].Params == nil {
		t.Fatal("multi.second should have non-nil Params")
	}

	// Verify initial values
	if wf.States[firstState.idx].Params["focus"] != "security" {
		t.Fatalf("multi.first Params[\"focus\"] = %q, want %q", wf.States[firstState.idx].Params["focus"], "security")
	}
	if wf.States[secondState.idx].Params["focus"] != "security" {
		t.Fatalf("multi.second Params[\"focus\"] = %q, want %q", wf.States[secondState.idx].Params["focus"], "security")
	}

	// Mutate first state's params — this should NOT affect second state
	wf.States[firstState.idx].Params["focus"] = "corrupted"

	// If params are aliased (shared reference), second state is also corrupted
	if wf.States[secondState.idx].Params["focus"] != "security" {
		t.Fatalf("params aliasing bug: modifying multi.first.Params also changed multi.second.Params[\"focus\"] to %q — states must have independent param copies",
			wf.States[secondState.idx].Params["focus"])
	}
}

type stateRef struct {
	idx int
}

// TestAdversary_ComposeSequentialParamsAliasingAcrossBlocks verifies that when
// two different resolved blocks happen to share the same underlying params map
// (e.g., built from the same source), mutations are isolated.
func TestAdversary_ComposeSequentialParamsAliasingAcrossBlocks(t *testing.T) {
	sharedParams := map[string]string{"mode": "strict"}

	blocks := []ResolvedBlock{
		{Name: "impl", Block: makeImplBlock(), Params: sharedParams},
		{Name: "check", Block: makeAdversaryBlock(), Params: sharedParams},
	}

	wf, err := ComposeSequential(blocks)
	if err != nil {
		t.Fatalf("ComposeSequential failed: %v", err)
	}

	// Find both states
	var implParams, checkParams map[string]string
	for _, s := range wf.States {
		if s.Name == "impl.impl" {
			implParams = s.Params
		}
		if s.Name == "check.adversary" {
			checkParams = s.Params
		}
	}

	if implParams == nil || checkParams == nil {
		t.Fatal("expected both states to have params")
	}

	// Mutate impl's params
	implParams["mode"] = "corrupted"

	// Check's params should be independent
	if checkParams["mode"] != "strict" {
		t.Fatalf("params aliasing across blocks: check.adversary Params[\"mode\"] = %q after mutating impl.impl — expected independent copies",
			checkParams["mode"])
	}
}

// TestAdversary_ComposePipelineParamsAliasingMultiState verifies the same
// aliasing issue exists via ComposePipeline with a multi-state block.
func TestAdversary_ComposePipelineParamsAliasingMultiState(t *testing.T) {
	multiBlock := &Block{
		Name:  "multi",
		Entry: "analyze",
		Exits: []string{"done"},
		States: []BlockState{
			{Name: "analyze", Prompt: "p1", Next: map[string]string{"": "report"}},
			{Name: "report", Prompt: "p2", Next: map[string]string{"": "$done"}},
		},
	}

	blockDefs := map[string]*Block{
		"multi": multiBlock,
	}

	steps := []PipelineStep{
		{Block: "multi", Name: "step1", Params: map[string]string{"focus": "performance"}},
	}

	wf, _, err := ComposePipeline(steps, blockDefs)
	if err != nil {
		t.Fatalf("ComposePipeline failed: %v", err)
	}

	// Find both states
	var analyzeParams, reportParams map[string]string
	for _, s := range wf.States {
		if s.Name == "step1.analyze" {
			analyzeParams = s.Params
		}
		if s.Name == "step1.report" {
			reportParams = s.Params
		}
	}

	if analyzeParams == nil || reportParams == nil {
		t.Fatal("expected both states to have params")
	}

	// Mutate analyze's params
	analyzeParams["focus"] = "corrupted"

	// Report's params should be independent
	if reportParams["focus"] != "performance" {
		t.Fatalf("params aliasing via ComposePipeline: step1.report Params[\"focus\"] = %q after mutating step1.analyze — expected independent copies",
			reportParams["focus"])
	}
}
