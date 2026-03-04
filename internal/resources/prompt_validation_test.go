package resources_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/workflow"
)

// workflowStateRef ties a prompt path back to the workflow state that references it.
type workflowStateRef struct {
	workflow string
	state    string
	verdicts []string
}

// allPromptPaths returns all embedded .md prompt paths via the exported ListPrompts.
func allPromptPaths(t *testing.T) []string {
	t.Helper()
	paths := resources.ListPrompts()
	if len(paths) == 0 {
		t.Fatal("no prompt files found in embedded resources")
	}
	return paths
}

// allWorkflowPromptRefs loads every workflow and returns a map from prompt path
// (relative, e.g. "bugfix/investigate.md") to the workflow states that reference it.
func allWorkflowPromptRefs(t *testing.T) map[string][]workflowStateRef {
	t.Helper()
	refs := make(map[string][]workflowStateRef)
	for _, name := range resources.ListWorkflows() {
		data, err := resources.WorkflowBytes(name)
		if err != nil {
			t.Fatalf("WorkflowBytes(%s): %v", name, err)
		}
		w, err := workflow.LoadBytes(data)
		if err != nil {
			t.Fatalf("LoadBytes(%s): %v", name, err)
		}
		for _, s := range w.States {
			if s.Prompt == "" {
				continue
			}
			// Workflow YAML stores "prompts/bugfix/investigate.md";
			// PromptBytes wants "bugfix/investigate.md".
			rel := strings.TrimPrefix(s.Prompt, "prompts/")
			refs[rel] = append(refs[rel], workflowStateRef{
				workflow: name,
				state:    s.Name,
				verdicts: s.Verdicts,
			})
		}
	}
	return refs
}

// validTemplateContext returns a TemplateContext with every field populated,
// so template rendering won't fail due to missing variables.
func validTemplateContext() prompt.TemplateContext {
	return prompt.TemplateContext{
		Phase:     "test-phase",
		Plan:      "test-plan",
		Iteration: 1,
		PlanMD:    "# Test Plan\n\nThis is a test plan.",
		State: map[string]string{
			"phase":                      "test-phase",
			"plan":                       "test-plan",
			"workflow_type":              "feature",
			"phase_status":               "active",
			"current_state":              "qa",
			"iteration":                  "1",
			"max_iterations":             "5",
			"tests_passing":              "10",
			"tests_total":                "10",
			"stuck_iterations":           "0",
			"hang_count":                 "0",
			"last_verdict":               "approved",
			"last_reviewed_iteration":    "0",
			"last_qa_reviewed_iteration": "0",
			"rollback_count":             "0",
			"global_iterations":          "0",
			"last_commit":                "abc1234",
			"model_override":             "",
		},
		Params: map[string]string{
			"test_command": "go test ./...",
			"language":     "go",
		},
		PlanFile:     ".plans/test-plan/plan.md",
		PhaseDir:     ".plans/test-plan/test-phase",
		StateFile:    ".plans/test-plan/test-phase/state.json",
		ScriptsDir:   ".arc/scripts",
		Mode:         "normal",
		DisputeCount: 0,
		DisputeList:  "(none)",
	}
}

// TestAllWorkflowPromptsExist verifies every prompt referenced by every workflow
// state actually exists in the embedded filesystem and is non-empty.
func TestAllWorkflowPromptsExist(t *testing.T) {
	refs := allWorkflowPromptRefs(t)
	if len(refs) == 0 {
		t.Fatal("no workflow prompt references found")
	}
	for promptPath, stateRefs := range refs {
		data, err := resources.PromptBytes(promptPath)
		if err != nil {
			t.Errorf("prompt %q (referenced by %s/%s) not found: %v",
				promptPath, stateRefs[0].workflow, stateRefs[0].state, err)
			continue
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			t.Errorf("prompt %q (referenced by %s/%s) is empty",
				promptPath, stateRefs[0].workflow, stateRefs[0].state)
		}
	}
}

// TestAllWorkflowPromptsRender verifies every workflow-referenced prompt renders
// without errors using a fully-populated template context.
func TestAllWorkflowPromptsRender(t *testing.T) {
	refs := allWorkflowPromptRefs(t)
	ctx := validTemplateContext()

	for promptPath, stateRefs := range refs {
		rendered, err := prompt.Render(promptPath, ctx)
		if err != nil {
			t.Errorf("prompt %q (referenced by %s/%s) failed to render: %v",
				promptPath, stateRefs[0].workflow, stateRefs[0].state, err)
			continue
		}
		if len(strings.TrimSpace(rendered)) == 0 {
			t.Errorf("prompt %q (referenced by %s/%s) rendered to empty string",
				promptPath, stateRefs[0].workflow, stateRefs[0].state)
		}
	}
}

// TestPartialIncludesResolve scans all prompt files for {{> path}} partial
// includes and verifies each partial exists and its content is inlined during rendering.
func TestPartialIncludesResolve(t *testing.T) {
	partialRe := regexp.MustCompile(`\{\{>\s+([^}]+?)\s*\}\}`)
	paths := allPromptPaths(t)
	ctx := validTemplateContext()

	for _, p := range paths {
		data, err := resources.PromptBytes(p)
		if err != nil {
			t.Fatalf("PromptBytes(%s): %v", p, err)
		}
		content := string(data)
		matches := partialRe.FindAllStringSubmatch(content, -1)
		if len(matches) == 0 {
			continue
		}

		for _, m := range matches {
			partialPath := m[1]

			// Verify the partial file exists.
			partialData, err := resources.PromptBytes(partialPath)
			if err != nil {
				t.Errorf("prompt %q includes partial %q which does not exist: %v", p, partialPath, err)
				continue
			}

			// Pick a marker string from the partial (first non-empty, non-template line).
			var marker string
			for _, line := range strings.Split(string(partialData), "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "{{") {
					continue
				}
				marker = trimmed
				break
			}
			if marker == "" {
				continue
			}

			// Render the parent and verify the partial's content was inlined.
			rendered, err := prompt.Render(p, ctx)
			if err != nil {
				t.Errorf("prompt %q failed to render (partial check): %v", p, err)
				continue
			}
			if !strings.Contains(rendered, marker) {
				t.Errorf("prompt %q includes partial %q but rendered output does not contain marker %q",
					p, partialPath, marker)
			}
		}
	}
}

// stripCodeBlocks removes fenced code blocks (``` ... ```) so that verdict
// headers inside templates/examples are not mistaken for real verdict sections.
func stripCodeBlocks(s string) string {
	var out strings.Builder
	inBlock := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
			continue
		}
		if !inBlock {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// TestVerdictConsistency checks that workflow states and their prompts agree on verdicts:
//   - States WITH verdicts: prompt must contain "## Verdict" and mention each verdict value.
//   - States WITHOUT verdicts (linear): prompt must NOT contain "## Verdict".
//
// Code blocks are stripped before checking so that verdict headers inside
// template examples (e.g., performance/benchmark.md) are not false positives.
func TestVerdictConsistency(t *testing.T) {
	refs := allWorkflowPromptRefs(t)

	for promptPath, stateRefs := range refs {
		data, err := resources.PromptBytes(promptPath)
		if err != nil {
			t.Fatalf("PromptBytes(%s): %v", promptPath, err)
		}
		content := stripCodeBlocks(string(data))
		hasVerdictSection := strings.Contains(content, "## Verdict")

		for _, ref := range stateRefs {
			// Skip fork states — parallel dispatchers that don't use prompts for agents.
			if strings.HasPrefix(ref.state, "_fork_") {
				continue
			}
			if len(ref.verdicts) > 0 {
				// Branching state: prompt must have verdict section.
				if !hasVerdictSection {
					t.Errorf("workflow %s state %s has verdicts %v but prompt %q has no '## Verdict' section",
						ref.workflow, ref.state, ref.verdicts, promptPath)
				}
				// Each verdict value should be mentioned in the prompt.
				for _, v := range ref.verdicts {
					if !strings.Contains(content, v) {
						t.Errorf("workflow %s state %s declares verdict %q but prompt %q does not mention it",
							ref.workflow, ref.state, v, promptPath)
					}
				}
			} else {
				// Linear state: prompt should not have verdict section.
				if hasVerdictSection {
					t.Errorf("workflow %s state %s is linear (no verdicts) but prompt %q contains '## Verdict' section",
						ref.workflow, ref.state, promptPath)
				}
			}
		}
	}
}

// TestNoOrphanPrompts verifies every prompt file is referenced by at least one
// workflow state, with exemptions for shared/special-purpose directories.
func TestNoOrphanPrompts(t *testing.T) {
	exemptDirs := []string{
		"common/",
		"adversaries/",
		"validate/",
		"dev/", // agent prompts loaded outside workflow states
	}
	exemptFiles := map[string]bool{
		"direct/multi-phase.md": true, // loaded by orchestrator for direct multi-phase plans
		"blocks/act.md":         true, // default prompt for act block
		"blocks/adversary.md":   true, // default prompt for adversary block
		"blocks/tests.md":       true, // default prompt for tests block
		"blocks/test-review.md": true, // default prompt for test-review block
		"blocks/review.md":      true, // default prompt for review block
	}

	refs := allWorkflowPromptRefs(t)
	paths := allPromptPaths(t)

	for _, p := range paths {
		exempted := false
		for _, prefix := range exemptDirs {
			if strings.HasPrefix(p, prefix) {
				exempted = true
				break
			}
		}
		if exempted || exemptFiles[p] {
			continue
		}

		if _, ok := refs[p]; !ok {
			t.Errorf("prompt %q is not referenced by any workflow state (orphan)", p)
		}
	}
}

// TestNoEmptyPromptFiles verifies every embedded prompt file has non-whitespace content.
// Known placeholder files are exempted.
func TestNoEmptyPromptFiles(t *testing.T) {
	exempt := map[string]bool{}

	paths := allPromptPaths(t)
	for _, p := range paths {
		if exempt[p] {
			continue
		}
		data, err := resources.PromptBytes(p)
		if err != nil {
			t.Errorf("PromptBytes(%s): %v", p, err)
			continue
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			t.Errorf("prompt %q is empty (no non-whitespace content)", p)
		}
	}
}
