package gate

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ---------------------------------------------------------------------------
// collectTestFiles
// ---------------------------------------------------------------------------

func TestCollectTestFiles_Empty(t *testing.T) {
	workdir := t.TempDir()

	// No files in the directory — expect empty result.
	files, err := collectTestFiles(workdir)
	if err != nil {
		t.Fatalf("collectTestFiles returned error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 test files, got %d: %v", len(files), files)
	}
}

func TestCollectTestFiles_FindsTestFiles(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "foo.go", "package foo\n")
	writeFile(t, workdir, "foo_test.go", "package foo\nfunc TestFoo(t *testing.T) {}\n")
	writeFile(t, workdir, "bar/bar_test.go", "package bar\nfunc TestBar(t *testing.T) {}\n")

	files, err := collectTestFiles(workdir)
	if err != nil {
		t.Fatalf("collectTestFiles returned error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 test files, got %d: %v", len(files), files)
	}
}

func TestCollectTestFiles_SkipsVendor(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "vendor/pkg/pkg_test.go", "package pkg\n")
	writeFile(t, workdir, "real_test.go", "package main\n")

	files, err := collectTestFiles(workdir)
	if err != nil {
		t.Fatalf("collectTestFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 test file (vendor excluded), got %d: %v", len(files), files)
	}
}

// ---------------------------------------------------------------------------
// RunSpecCoverage
// ---------------------------------------------------------------------------

func TestRunSpecCoverage_EmptySpec_ReturnsPass(t *testing.T) {
	workdir := t.TempDir()

	// No assertions have SpecCoverage set — should return nil.
	assertions := []arc.GateAssertion{
		{FileExists: "main.go"},
		{Grep: "package main"},
	}
	results := RunSpecCoverage(assertions, workdir)
	if results != nil {
		t.Errorf("expected nil results for no spec_coverage assertions, got %v", results)
	}
}

func TestRunSpecCoverage_NoAssertions(t *testing.T) {
	workdir := t.TempDir()

	results := RunSpecCoverage(nil, workdir)
	if results != nil {
		t.Errorf("expected nil results for nil assertions, got %v", results)
	}
}

func TestRunSpecCoverage_Pass(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "foo_test.go", "package foo\nfunc TestFoo(t *testing.T) {}\n")

	assertions := []arc.GateAssertion{
		{SpecCoverage: "TestFoo"},
	}
	results := RunSpecCoverage(assertions, workdir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected pass, got: %s", results[0].Detail)
	}
}

func TestRunSpecCoverage_Fail(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "foo_test.go", "package foo\nfunc TestFoo(t *testing.T) {}\n")

	assertions := []arc.GateAssertion{
		{SpecCoverage: "TestMissing"},
	}
	results := RunSpecCoverage(assertions, workdir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected fail for missing target")
	}
}

func TestRunSpecCoverage_UsesDescription(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "foo_test.go", "package foo\nfunc TestFoo(t *testing.T) {}\n")

	desc := "custom description"
	assertions := []arc.GateAssertion{
		{Description: desc, SpecCoverage: "TestFoo"},
	}
	results := RunSpecCoverage(assertions, workdir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Description != desc {
		t.Errorf("expected description %q, got %q", desc, results[0].Description)
	}
}

func TestRunSpecCoverage_DefaultDescription(t *testing.T) {
	workdir := t.TempDir()

	assertions := []arc.GateAssertion{
		{SpecCoverage: "MyFunc"},
	}
	results := RunSpecCoverage(assertions, workdir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Description == "" {
		t.Error("expected non-empty default description")
	}
}
