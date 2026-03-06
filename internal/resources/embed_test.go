package resources

import (
	"testing"
)

func TestEmbeddedPromptAccessible(t *testing.T) {
	data, err := PromptBytes("gate/impl.md")
	if err != nil {
		t.Fatalf("PromptBytes(gate/impl.md) error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PromptBytes(gate/impl.md) returned empty byte slice")
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
