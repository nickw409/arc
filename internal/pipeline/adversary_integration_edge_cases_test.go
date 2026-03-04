package pipeline

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
)

// ── Adversary integration tests: RunParallel prompt resolution ────────────────

// TestAdversary_RunParallelBranchPromptContentMissing verifies that when
// RunParallel renders a branch prompt using prompt.RenderString(b.Prompt, ...),
// where b.Prompt is a resource path like "prompts/blocks/adversary.md", the
// output does NOT contain the expected adversary prompt sections like
// "## Instructions", "## Verdict", "Test Execution Rules", etc.
//
// This proves a critical bug: branch agents receive a useless prompt (just the
// file path text) instead of the full adversary testing instructions. RunParallel
// should load the prompt content from embedded resources before template rendering.
func TestAdversary_RunParallelBranchPromptContentMissing(t *testing.T) {
	// Simulate RunParallel's prompt rendering for a parallel branch.
	// ComposePipeline stores the adversary block's entry state prompt path
	// (e.g. "prompts/blocks/adversary.md") as branch.Prompt.
	branchPrompt := "prompts/blocks/adversary.md"

	ctx := prompt.TemplateContext{
		Phase:      "test-phase",
		Plan:       "test-plan",
		Iteration:  1,
		PlanMD:     "# Phase Spec\nImplement X",
		State:      map[string]string{"iteration": "1"},
		Params:     map[string]string{"focus": "core functionality"},
		PlanFile:   ".plans/test-plan/plan.md",
		PhaseDir:   ".plans/test-plan/phases/test-phase",
		StateFile:  ".plans/test-plan/phases/test-phase/state.json",
		ScriptsDir: ".arc/scripts",
	}

	// This is exactly what RunParallel does at parallel.go:94
	rendered, err := prompt.RenderString(branchPrompt, ctx)
	if err != nil {
		t.Fatalf("RenderString failed: %v", err)
	}

	// The actual adversary prompt should contain these key sections.
	// If RunParallel correctly loaded the prompt, all would be present.
	requiredSections := []string{
		"Instructions",
		"Verdict",
		"Test Execution Rules",
	}

	for _, section := range requiredSections {
		if !strings.Contains(rendered, section) {
			t.Errorf("rendered branch prompt missing expected section %q — "+
				"RunParallel renders the resource path as a literal template "+
				"instead of loading the actual prompt content.\n"+
				"Rendered (%d chars): %q",
				section, len(rendered), rendered)
		}
	}
}

// TestAdversary_RunParallelBranchPromptTooShort verifies that the prompt
// produced by RunParallel's template rendering of a branch prompt path is
// absurdly short — just the file path text, not the full multi-KB adversary
// prompt. A properly loaded adversary prompt should be at least 500 characters.
func TestAdversary_RunParallelBranchPromptTooShort(t *testing.T) {
	branchPrompt := "prompts/blocks/adversary.md"

	ctx := prompt.TemplateContext{
		Phase:      "test-phase",
		Plan:       "test-plan",
		Iteration:  1,
		PlanMD:     "# Test",
		State:      map[string]string{"iteration": "1"},
		Params:     map[string]string{"focus": "edge cases"},
		PlanFile:   ".plans/test/plan.md",
		PhaseDir:   ".plans/test/phases/test",
		StateFile:  ".plans/test/phases/test/state.json",
		ScriptsDir: ".arc/scripts",
	}

	rendered, err := prompt.RenderString(branchPrompt, ctx)
	if err != nil {
		t.Fatalf("RenderString failed: %v", err)
	}

	// The actual adversary prompt (blocks/adversary.md) is ~100 lines.
	// When properly loaded and rendered, it should be at least 500 characters.
	// But since RunParallel only renders the path string, the output is ~30 chars.
	if len(rendered) < 500 {
		t.Fatalf("rendered branch prompt is only %d characters — "+
			"this is just the resource path text, not the actual prompt.\n"+
			"RunParallel should load prompt content from resources before rendering.\n"+
			"Rendered: %q", len(rendered), rendered)
	}
}

// TestAdversary_RunParallelPromptPathNotRendered verifies the exact bug:
// prompt.RenderString called with a resource path produces a literal path
// string. The non-parallel RunState path (iterate.go:228-255) correctly
// loads the prompt bytes first, but RunParallel (parallel.go:94) doesn't.
func TestAdversary_RunParallelPromptPathNotRendered(t *testing.T) {
	// What RunParallel does (WRONG):
	branchPrompt := "prompts/blocks/adversary.md"
	ctx := prompt.TemplateContext{
		Phase: "test", Plan: "test", Iteration: 1,
		PlanMD: "# Test", State: map[string]string{"iteration": "1"},
		Params: map[string]string{}, PlanFile: "plan.md",
		PhaseDir: "phases/test", StateFile: "state.json", ScriptsDir: "scripts",
	}

	wrongRendered, err := prompt.RenderString(branchPrompt, ctx)
	if err != nil {
		t.Fatalf("wrong path: %v", err)
	}

	// What RunParallel SHOULD do (CORRECT):
	promptPath := strings.TrimPrefix(branchPrompt, "prompts/")
	correctRendered, err := prompt.Render(promptPath, ctx)
	if err != nil {
		t.Fatalf("correct path: %v", err)
	}

	// The wrong approach produces just the path text.
	// The correct approach produces the full prompt content.
	if len(wrongRendered) >= len(correctRendered) {
		// If wrongRendered is longer, it somehow loaded the content (fixed!)
		return
	}

	// This should fail: the wrong approach produces drastically less content
	ratio := float64(len(wrongRendered)) / float64(len(correctRendered))
	if ratio < 0.1 {
		t.Fatalf("RunParallel produces %.1f%% of the expected prompt content.\n"+
			"Wrong (path only): %d chars\n"+
			"Correct (loaded):  %d chars\n\n"+
			"RunParallel at parallel.go:94 calls prompt.RenderString(b.Prompt, ...) "+
			"where b.Prompt is a resource path. It should call prompt.Render() or "+
			"load resources.PromptBytes() first, like RunState does at iterate.go:228-255.",
			ratio*100, len(wrongRendered), len(correctRendered))
	}
}

// TestAdversary_MergeVerdictsPositiveVerdictFromFeatureWorkflow verifies the
// exact merge behavior that the feature workflow depends on.
func TestAdversary_MergeVerdictsPositiveVerdictFromFeatureWorkflow(t *testing.T) {
	validVerdicts := []arc.Verdict{"bugs_found", "no_bugs_found"}
	positiveVerdict := arc.Verdict("no_bugs_found")

	// Case 1: Both agree on no_bugs_found
	v, err := MergeVerdicts("all",
		map[string]arc.Verdict{"check-a": "no_bugs_found", "check-b": "no_bugs_found"},
		validVerdicts, positiveVerdict)
	if err != nil {
		t.Fatalf("case 1: %v", err)
	}
	if v != "no_bugs_found" {
		t.Fatalf("case 1: both no_bugs_found should merge to no_bugs_found, got %q", v)
	}

	// Case 2: Both agree on bugs_found
	v, err = MergeVerdicts("all",
		map[string]arc.Verdict{"check-a": "bugs_found", "check-b": "bugs_found"},
		validVerdicts, positiveVerdict)
	if err != nil {
		t.Fatalf("case 2: %v", err)
	}
	if v != "bugs_found" {
		t.Fatalf("case 2: both bugs_found should merge to bugs_found, got %q", v)
	}

	// Case 3: Mixed — check-a finds bugs, check-b doesn't
	v, err = MergeVerdicts("all",
		map[string]arc.Verdict{"check-a": "bugs_found", "check-b": "no_bugs_found"},
		validVerdicts, positiveVerdict)
	if err != nil {
		t.Fatalf("case 3: %v", err)
	}
	if v != "bugs_found" {
		t.Fatalf("case 3: mixed verdicts under 'all' strategy with positiveVerdict=%q "+
			"should return the negative verdict 'bugs_found', got %q",
			positiveVerdict, v)
	}

	// Case 4: Mixed — check-a clean, check-b finds bugs
	v, err = MergeVerdicts("all",
		map[string]arc.Verdict{"check-a": "no_bugs_found", "check-b": "bugs_found"},
		validVerdicts, positiveVerdict)
	if err != nil {
		t.Fatalf("case 4: %v", err)
	}
	if v != "bugs_found" {
		t.Fatalf("case 4: mixed verdicts should return bugs_found regardless of branch order, got %q", v)
	}
}

// TestAdversary_FeatureWorkflowRunOnceSkipVerdictMatchesMergedVerdict verifies
// that the skip_exit value ("no_bugs_found") on the feature workflow's parallel
// step is a valid verdict that MergeVerdicts can actually produce.
func TestAdversary_FeatureWorkflowRunOnceSkipVerdictMatchesMergedVerdict(t *testing.T) {
	validVerdicts := []arc.Verdict{"bugs_found", "no_bugs_found"}

	v, err := MergeVerdicts("all",
		map[string]arc.Verdict{"check-a": "no_bugs_found", "check-b": "no_bugs_found"},
		validVerdicts, "no_bugs_found")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skipExit := "no_bugs_found"
	if string(v) != skipExit {
		t.Fatalf("MergeVerdicts produced %q but skip_exit expects %q — "+
			"run_once will never activate", v, skipExit)
	}
}

// TestAdversary_MapStateToStatusForForkState verifies that MapStateToStatus
// correctly maps fork state names (like "_fork_0") to a status.
func TestAdversary_MapStateToStatusForForkState(t *testing.T) {
	status := MapStateToStatus("_fork_0")
	if status != "_fork_0" {
		t.Fatalf("MapStateToStatus(%q) = %q, want %q", "_fork_0", status, "_fork_0")
	}
	if status == "adversary" {
		t.Fatal("fork state should NOT map to 'adversary' status")
	}
}

// TestAdversary_MapStateToStatusForkNotAdversary verifies that parallel fork
// states are NOT mapped to "adversary" status.
func TestAdversary_MapStateToStatusForkNotAdversary(t *testing.T) {
	forkStates := []string{"_fork_0", "_fork_1", "_fork_99"}
	for _, name := range forkStates {
		status := MapStateToStatus(name)
		if status == "adversary" {
			t.Errorf("MapStateToStatus(%q) = %q — fork states should NOT be mapped "+
				"to 'adversary' status", name, status)
		}
	}
}

// TestAdversary_FeatureWorkflowBranchPromptMatchesEmbedded verifies that the
// branch prompt stored in the feature workflow's parallel config actually
// points to an existing embedded resource. If the path is wrong, RunParallel
// won't be able to load it (once the prompt loading bug is fixed).
func TestAdversary_FeatureWorkflowBranchPromptMatchesEmbedded(t *testing.T) {
	// The adversary block stores its prompt as something like
	// "prompts/blocks/adversary.md". After stripping the "prompts/" prefix,
	// it should be loadable via resources.PromptBytes().
	possiblePaths := []string{
		"blocks/adversary.md",
		"adversarial/adversary.md",
	}

	foundValid := false
	for _, path := range possiblePaths {
		data, err := resources.PromptBytes(path)
		if err == nil && len(data) > 0 {
			foundValid = true
			break
		}
	}

	if !foundValid {
		t.Fatal("no valid adversary prompt found at expected embedded paths — " +
			"RunParallel won't be able to load the prompt even after the path " +
			"rendering bug is fixed")
	}
}
