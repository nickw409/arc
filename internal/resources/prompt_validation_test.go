package resources_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
)

// allPromptPaths returns all embedded .md prompt paths via the exported ListPrompts.
func allPromptPaths(t *testing.T) []string {
	t.Helper()
	paths, err := resources.ListPrompts()
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no prompt files found in embedded resources")
	}
	return paths
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

// TestNoEmptyPromptFiles verifies every embedded prompt file has non-whitespace content.
func TestNoEmptyPromptFiles(t *testing.T) {
	paths := allPromptPaths(t)
	for _, p := range paths {
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
