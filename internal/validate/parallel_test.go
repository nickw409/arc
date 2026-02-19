package validate

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestMergeReportsEmpty(t *testing.T) {
	merged := MergeReports(nil)
	if merged.Verdict != "pass" {
		t.Fatalf("Verdict = %q, want %q", merged.Verdict, "pass")
	}
	if len(merged.Findings) != 0 {
		t.Fatalf("len(Findings) = %d, want 0", len(merged.Findings))
	}
	if merged.Summary.Critical != 0 {
		t.Fatalf("Critical = %d, want 0", merged.Summary.Critical)
	}
}

func TestMergeReportsSingle(t *testing.T) {
	r := &Report{
		Findings: []Finding{
			{Severity: SeverityWarning, Location: "a.go:1", Category: "Test", Description: "warn"},
		},
		Summary: Summary{FilesAudited: 3, Warning: 1},
		Verdict: "pass",
	}

	merged := MergeReports([]*Report{r})
	if merged.Verdict != "pass" {
		t.Fatalf("Verdict = %q, want %q", merged.Verdict, "pass")
	}
	if len(merged.Findings) != 1 {
		t.Fatalf("len(Findings) = %d, want 1", len(merged.Findings))
	}
	if merged.Summary.FilesAudited != 3 {
		t.Fatalf("FilesAudited = %d, want 3", merged.Summary.FilesAudited)
	}
	if merged.Summary.Warning != 1 {
		t.Fatalf("Warning = %d, want 1", merged.Summary.Warning)
	}
}

func TestMergeReportsMultiple(t *testing.T) {
	r1 := &Report{
		Findings: []Finding{
			{Severity: SeverityWarning, Location: "a.go:1", Category: "Test", Description: "warn1"},
		},
		Summary:   Summary{FilesAudited: 2, Warning: 1},
		Verdict:   "pass",
		RawOutput: "report1",
	}
	r2 := &Report{
		Findings: []Finding{
			{Severity: SeverityInfo, Location: "b.go:5", Category: "Coverage", Description: "info1"},
			{Severity: SeverityWarning, Location: "b.go:10", Category: "Test", Description: "warn2"},
		},
		Summary:   Summary{FilesAudited: 4, Warning: 1, Info: 1},
		Verdict:   "pass",
		RawOutput: "report2",
	}

	merged := MergeReports([]*Report{r1, r2})
	if merged.Verdict != "pass" {
		t.Fatalf("Verdict = %q, want %q", merged.Verdict, "pass")
	}
	if len(merged.Findings) != 3 {
		t.Fatalf("len(Findings) = %d, want 3", len(merged.Findings))
	}
	if merged.Summary.FilesAudited != 6 {
		t.Fatalf("FilesAudited = %d, want 6", merged.Summary.FilesAudited)
	}
	if merged.Summary.Warning != 2 {
		t.Fatalf("Warning = %d, want 2", merged.Summary.Warning)
	}
	if merged.Summary.Info != 1 {
		t.Fatalf("Info = %d, want 1", merged.Summary.Info)
	}
}

func TestMergeReportsCriticalPropagates(t *testing.T) {
	r1 := &Report{
		Findings: []Finding{
			{Severity: SeverityWarning, Location: "a.go:1", Category: "Test", Description: "warn"},
		},
		Summary: Summary{FilesAudited: 1, Warning: 1},
		Verdict: "pass",
	}
	r2 := &Report{
		Findings: []Finding{
			{Severity: SeverityCritical, Location: "b.go:5", Category: "Gamed", Description: "crit"},
		},
		Summary: Summary{FilesAudited: 1, Critical: 1},
		Verdict: "fail",
	}

	merged := MergeReports([]*Report{r1, r2})
	if merged.Verdict != "fail" {
		t.Fatalf("Verdict = %q, want %q", merged.Verdict, "fail")
	}
	if merged.Summary.Critical != 1 {
		t.Fatalf("Critical = %d, want 1", merged.Summary.Critical)
	}
}

func TestFormatFileContents(t *testing.T) {
	files := []FileEntry{
		{Path: "pkg/foo.go", Content: "package pkg\n", IsTest: false},
		{Path: "pkg/foo_test.go", Content: "package pkg\n", IsTest: true},
	}

	result := formatFileContents(files)

	if !strings.Contains(result, "### `pkg/foo.go` (source)") {
		t.Fatal("missing source file header")
	}
	if !strings.Contains(result, "### `pkg/foo_test.go` (test)") {
		t.Fatal("missing test file header")
	}
	if !strings.Contains(result, "```\npackage pkg\n```") {
		t.Fatal("missing fenced code block")
	}
}

func TestRenderBatchPrompt(t *testing.T) {
	tmpl := "Audit {{package}}\n\n{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}\n\n{{files}}"

	batch := Batch{
		Package: "internal/foo",
		Files: []FileEntry{
			{Path: "internal/foo/bar.go", Content: "package foo\n", IsTest: false},
		},
	}

	result := renderBatchPrompt(tmpl, batch, "go")

	if !strings.Contains(result, "Audit internal/foo") {
		t.Fatal("package not substituted")
	}
	if !strings.Contains(result, "**go**") {
		t.Fatal("language not substituted")
	}
	if !strings.Contains(result, "### `internal/foo/bar.go` (source)") {
		t.Fatal("files not substituted")
	}
}

func TestRenderBatchPromptNoLanguage(t *testing.T) {
	tmpl := "Audit {{package}}\n\n{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}\n\n{{files}}"

	batch := Batch{
		Package: "pkg",
		Files:   []FileEntry{{Path: "pkg/a.go", Content: "pkg\n"}},
	}

	result := renderBatchPrompt(tmpl, batch, "")
	if strings.Contains(result, "{{language}}") {
		t.Fatal("language placeholder not removed")
	}
	if strings.Contains(result, "{{#if language}}") {
		t.Fatal("language conditional not removed")
	}
}

func TestRunParallelContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.Level(99)}))

	_, err := RunParallel(ctx, ParallelOptions{
		Batches: []Batch{{Package: "pkg", Files: []FileEntry{{Path: "a.go", Lines: 1}}}},
		Logger:  logger,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
