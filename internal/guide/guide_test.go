package guide

import (
	"strings"
	"testing"
)

func TestRenderFullGuide(t *testing.T) {
	data, err := Render("")
	if err != nil {
		t.Fatalf("Render('') error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Render('') returned empty output")
	}
	content := string(data)
	if strings.Contains(content, "<!-- section:") || strings.Contains(content, "<!-- /section:") {
		t.Fatal("full guide output contains raw section markers")
	}
}

func TestRenderSetupSection(t *testing.T) {
	data, err := Render("setup")
	if err != nil {
		t.Fatalf("Render('setup') error: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "arc init") {
		t.Fatal("setup section does not contain 'arc init'")
	}
}

func TestRenderPlansSection(t *testing.T) {
	data, err := Render("plans")
	if err != nil {
		t.Fatalf("Render('plans') error: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "plan.md") {
		t.Fatal("plans section does not contain 'plan.md'")
	}
}

func TestRenderExecutionSection(t *testing.T) {
	data, err := Render("execution")
	if err != nil {
		t.Fatalf("Render('execution') error: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "arc run") {
		t.Fatal("execution section does not contain 'arc run'")
	}
}

func TestRenderInvalidSection(t *testing.T) {
	_, err := Render("nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid section, got nil")
	}
}

func TestValidSections(t *testing.T) {
	for _, s := range ValidSections() {
		_, err := Render(s)
		if err != nil {
			t.Fatalf("ValidSections() includes %q but Render(%q) failed: %v", s, s, err)
		}
	}
}
