package resources

import (
	"sort"
	"strings"
	"testing"
)

func TestEmbeddedWorkflowsAccessible(t *testing.T) {
	data, err := WorkflowBytes("feature")
	if err != nil {
		t.Fatalf("WorkflowBytes(feature) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("WorkflowBytes(feature) returned empty byte slice")
	}
}

func TestEmbeddedWorkflowNotFound(t *testing.T) {
	_, err := WorkflowBytes("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workflow, got nil")
	}
}

func TestEmbeddedAllWorkflowTypes(t *testing.T) {
	workflows := ListWorkflows()
	expected := []string{"adversarial", "audit", "bugfix", "direct", "feature", "investigation", "performance", "refactor"}

	sort.Strings(workflows)
	if len(workflows) != len(expected) {
		t.Fatalf("ListWorkflows() = %v, want %v", workflows, expected)
	}
	for i, name := range expected {
		if workflows[i] != name {
			t.Fatalf("ListWorkflows()[%d] = %q, want %q", i, workflows[i], name)
		}
	}
}

func TestEmbeddedPromptAccessible(t *testing.T) {
	data, err := PromptBytes("feature/qa.md")
	if err != nil {
		t.Fatalf("PromptBytes(feature/qa.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PromptBytes(feature/qa.md) returned empty byte slice")
	}
}

func TestEmbeddedPromptNotFound(t *testing.T) {
	_, err := PromptBytes("nonexistent/prompt.md")
	if err == nil {
		t.Fatal("expected error for nonexistent prompt, got nil")
	}
}

func TestEmbeddedTemplateAccessible(t *testing.T) {
	data, err := TemplateBytes("plan-template.md")
	if err != nil {
		t.Fatalf("TemplateBytes(plan-template.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("TemplateBytes(plan-template.md) returned empty byte slice")
	}
}

func TestEmbeddedTemplateNotFound(t *testing.T) {
	_, err := TemplateBytes("nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent template, got nil")
	}
}

func TestEmbeddedHookAccessible(t *testing.T) {
	data, err := HookBytes("pre-commit")
	if err != nil {
		t.Fatalf("HookBytes(pre-commit) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("HookBytes(pre-commit) returned empty byte slice")
	}
}

func TestHookPathTraversal(t *testing.T) {
	_, err := HookBytes("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestResourcePathTraversalAll(t *testing.T) {
	traversalPath := "../../../etc/passwd"

	_, err := WorkflowBytes(traversalPath)
	if err == nil {
		t.Fatal("WorkflowBytes should reject path traversal")
	}

	_, err = PromptBytes(traversalPath)
	if err == nil {
		t.Fatal("PromptBytes should reject path traversal")
	}

	_, err = TemplateBytes(traversalPath)
	if err == nil {
		t.Fatal("TemplateBytes should reject path traversal")
	}
}

func TestEmbeddedValidatePromptAccessible(t *testing.T) {
	data, err := PromptBytes("validate/audit.md")
	if err != nil {
		t.Fatalf("PromptBytes(validate/audit.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PromptBytes(validate/audit.md) returned empty byte slice")
	}
}

func TestEmbeddedBatchAuditPromptAccessible(t *testing.T) {
	data, err := PromptBytes("validate/batch-audit.md")
	if err != nil {
		t.Fatalf("PromptBytes(validate/batch-audit.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PromptBytes(validate/batch-audit.md) returned empty byte slice")
	}
}

func TestEmbeddedGuideAccessible(t *testing.T) {
	data, err := GuideBytes("guide.md")
	if err != nil {
		t.Fatalf("GuideBytes(guide.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GuideBytes(guide.md) returned empty byte slice")
	}
}

func TestEmbeddedGuideNotFound(t *testing.T) {
	_, err := GuideBytes("nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent guide, got nil")
	}
}

func TestEmbeddedWorkflowContent(t *testing.T) {
	// Verify embedded workflows have expected content markers
	types := []string{"feature", "bugfix", "investigation", "refactor", "performance"}
	for _, wt := range types {
		data, err := WorkflowBytes(wt)
		if err != nil {
			t.Fatalf("WorkflowBytes(%s) error: %v", wt, err)
		}
		content := string(data)
		if !strings.Contains(content, "name:") {
			t.Fatalf("WorkflowBytes(%s) missing 'name:' field", wt)
		}
		if !strings.Contains(content, "states:") {
			t.Fatalf("WorkflowBytes(%s) missing 'states:' field", wt)
		}
	}
}
