package validate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestParseReportPass(t *testing.T) {
	output := `## Findings
### CRITICAL
None

### WARNING
- [api/handler.go:42] Assertion Completeness: Test checks error is nil but never verifies response body

### INFO
- [api/handler.go:10] Coverage Gaps: No test for the timeout branch

## Summary
- Files audited: 3
- Critical: 0, Warning: 1, Info: 1

## Verdict
pass`

	report, err := ParseReport(output)
	if err != nil {
		t.Fatalf("ParseReport() error: %v", err)
	}
	if report.Verdict != "pass" {
		t.Fatalf("Verdict = %q, want %q", report.Verdict, "pass")
	}
	if report.HasCritical() {
		t.Fatal("HasCritical() = true, want false")
	}
	if len(report.Findings) != 2 {
		t.Fatalf("len(Findings) = %d, want 2", len(report.Findings))
	}
	if report.Summary.Warning != 1 {
		t.Fatalf("Summary.Warning = %d, want 1", report.Summary.Warning)
	}
	if report.Summary.Info != 1 {
		t.Fatalf("Summary.Info = %d, want 1", report.Summary.Info)
	}
	if report.Summary.FilesAudited != 3 {
		t.Fatalf("Summary.FilesAudited = %d, want 3", report.Summary.FilesAudited)
	}
}

func TestParseReportFail(t *testing.T) {
	output := `## Findings
### CRITICAL
- [tests/test_auth.go:15] Gamed Tests: Test always passes regardless of implementation
- [tests/test_auth.go:30] Assertion Completeness: Function called but return value ignored

### WARNING
- [tests/test_auth.go:55] Mock Fidelity: Mock returns hardcoded success

### INFO
None

## Summary
- Files audited: 1
- Critical: 2, Warning: 1, Info: 0

## Verdict
fail`

	report, err := ParseReport(output)
	if err != nil {
		t.Fatalf("ParseReport() error: %v", err)
	}
	if report.Verdict != "fail" {
		t.Fatalf("Verdict = %q, want %q", report.Verdict, "fail")
	}
	if !report.HasCritical() {
		t.Fatal("HasCritical() = false, want true")
	}
	if report.Summary.Critical != 2 {
		t.Fatalf("Summary.Critical = %d, want 2", report.Summary.Critical)
	}
	if len(report.Findings) != 3 {
		t.Fatalf("len(Findings) = %d, want 3", len(report.Findings))
	}
}

func TestParseReportNoVerdict(t *testing.T) {
	output := `## Findings
### CRITICAL
None

### WARNING
None

### INFO
None

## Summary
- Files audited: 1
- Critical: 0, Warning: 0, Info: 0`

	_, err := ParseReport(output)
	if err == nil {
		t.Fatal("expected error for missing verdict, got nil")
	}
	if got := err.Error(); got != "no verdict section found" {
		t.Fatalf("error = %q, want %q", got, "no verdict section found")
	}
}

func TestParseReportInvalidVerdict(t *testing.T) {
	output := `## Findings
### CRITICAL
None

## Summary
- Files audited: 1
- Critical: 0, Warning: 0, Info: 0

## Verdict
maybe`

	_, err := ParseReport(output)
	if err == nil {
		t.Fatal("expected error for invalid verdict, got nil")
	}
	if got := err.Error(); got != `invalid verdict value: "maybe"` {
		t.Fatalf("error = %q, want %q", got, `invalid verdict value: "maybe"`)
	}
}

func TestParseReportEmptyFindings(t *testing.T) {
	output := `## Findings
### CRITICAL
None

### WARNING
None

### INFO
None

## Summary
- Files audited: 5
- Critical: 0, Warning: 0, Info: 0

## Verdict
pass`

	report, err := ParseReport(output)
	if err != nil {
		t.Fatalf("ParseReport() error: %v", err)
	}
	if report.Verdict != "pass" {
		t.Fatalf("Verdict = %q, want %q", report.Verdict, "pass")
	}
	if len(report.Findings) != 0 {
		t.Fatalf("len(Findings) = %d, want 0", len(report.Findings))
	}
	if report.Summary.FilesAudited != 5 {
		t.Fatalf("Summary.FilesAudited = %d, want 5", report.Summary.FilesAudited)
	}
}

func TestParseFindingLine(t *testing.T) {
	tests := []struct {
		line     string
		wantOK   bool
		location string
		category string
		desc     string
	}{
		{
			line:     "- [api/handler.go:42] Assertion Completeness: Test never checks return value",
			wantOK:   true,
			location: "api/handler.go:42",
			category: "Assertion Completeness",
			desc:     "Test never checks return value",
		},
		{
			line:     "- [test.py:1] Gamed Tests: Always passes",
			wantOK:   true,
			location: "test.py:1",
			category: "Gamed Tests",
			desc:     "Always passes",
		},
		{
			line:   "None",
			wantOK: false,
		},
		{
			line:   "some random text",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		f, ok := parseFindingLine(tt.line, SeverityCritical)
		if ok != tt.wantOK {
			t.Errorf("parseFindingLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if f.Location != tt.location {
			t.Errorf("Location = %q, want %q", f.Location, tt.location)
		}
		if f.Category != tt.category {
			t.Errorf("Category = %q, want %q", f.Category, tt.category)
		}
		if f.Description != tt.desc {
			t.Errorf("Description = %q, want %q", f.Description, tt.desc)
		}
		if f.Severity != SeverityCritical {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityCritical)
		}
	}
}

func TestTryLoadConfigNoConfig(t *testing.T) {
	dir := t.TempDir()
	pc := TryLoadConfig(dir)
	if pc.Language != "" {
		t.Fatalf("Language = %q, want %q", pc.Language, "")
	}
	if pc.PromptPath != "" {
		t.Fatalf("PromptPath = %q, want %q", pc.PromptPath, "")
	}
}

func TestTryLoadConfigLanguageOnly(t *testing.T) {
	dir := t.TempDir()
	cfg := []byte("language: go\nrunner: go-test\n")
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	pc := TryLoadConfig(dir)
	if pc.Language != "go" {
		t.Fatalf("Language = %q, want %q", pc.Language, "go")
	}
	if pc.PromptPath != "" {
		t.Fatalf("PromptPath = %q, want %q", pc.PromptPath, "")
	}
}

func TestTryLoadConfigWithPrompt(t *testing.T) {
	dir := t.TempDir()
	cfg := []byte("language: go\nrunner: go-test\naudit:\n  prompt: my-audit.md\n")
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	pc := TryLoadConfig(dir)
	if pc.Language != "go" {
		t.Fatalf("Language = %q, want %q", pc.Language, "go")
	}
	want := filepath.Join(dir, "my-audit.md")
	if pc.PromptPath != want {
		t.Fatalf("PromptPath = %q, want %q", pc.PromptPath, want)
	}
}

func TestTryLoadConfigAbsolutePromptPath(t *testing.T) {
	dir := t.TempDir()
	cfg := []byte("language: go\nrunner: go-test\naudit:\n  prompt: /tmp/custom-audit.md\n")
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	pc := TryLoadConfig(dir)
	if pc.PromptPath != "/tmp/custom-audit.md" {
		t.Fatalf("PromptPath = %q, want %q", pc.PromptPath, "/tmp/custom-audit.md")
	}
}

func TestLoadPromptCustomFile(t *testing.T) {
	dir := t.TempDir()
	content := "custom prompt content"
	path := filepath.Join(dir, "audit.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := loadPrompt(path)
	if err != nil {
		t.Fatalf("loadPrompt() error: %v", err)
	}
	if string(data) != content {
		t.Fatalf("loadPrompt() = %q, want %q", string(data), content)
	}
}

func TestLoadPromptCustomFileMissing(t *testing.T) {
	_, err := loadPrompt("/nonexistent/audit.md")
	if err == nil {
		t.Fatal("expected error for missing custom prompt, got nil")
	}
}

func TestLoadPromptDefault(t *testing.T) {
	data, err := loadPrompt("")
	if err != nil {
		t.Fatalf("loadPrompt() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("loadPrompt() returned empty bytes for default prompt")
	}
}

func TestRunContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, Options{
		Paths:  []string{"."},
		Logger: noopLogger(),
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.Level(99)}))
}
