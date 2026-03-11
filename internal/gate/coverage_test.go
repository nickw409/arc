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
// RunSpecCoverage — early-exit cases (no agent spawned)
// ---------------------------------------------------------------------------

func TestRunSpecCoverage_NoSpecCoverageAssertions_ReturnsNil(t *testing.T) {
	workdir := t.TempDir()
	assertions := []arc.GateAssertion{
		{FileExists: "main.go"},
		{Grep: "package main"},
	}
	results := RunSpecCoverage(t.Context(), &arc.PhaseSpec{}, assertions, workdir, "")
	if results != nil {
		t.Errorf("expected nil results for no spec_coverage assertions, got %v", results)
	}
}

func TestRunSpecCoverage_NilAssertions_ReturnsNil(t *testing.T) {
	workdir := t.TempDir()
	results := RunSpecCoverage(t.Context(), &arc.PhaseSpec{}, nil, workdir, "")
	if results != nil {
		t.Errorf("expected nil results for nil assertions, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// parseSpecCoverageOutput — unit tests for the verdict parser
// ---------------------------------------------------------------------------

func TestParseSpecCoverageOutput_Pass(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "func NewFoo"}}
	output := "PASS 1: func NewFoo — tested in TestNewFoo\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected pass, got detail: %s", results[0].Detail)
	}
}

func TestParseSpecCoverageOutput_Fail(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "func Missing"}}
	output := "FAIL 1: func Missing — no test exercises this function\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected fail")
	}
}

func TestParseSpecCoverageOutput_MultipleTargets(t *testing.T) {
	assertions := []arc.GateAssertion{
		{SpecCoverage: "func Foo"},
		{SpecCoverage: "func Bar"},
		{SpecCoverage: "func Baz"},
	}
	output := "PASS 1: func Foo — covered in TestFoo\nFAIL 2: func Bar — no test found\nPASS 3: func Baz — covered in TestBaz\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Error("expected result[0] pass")
	}
	if results[1].Passed {
		t.Error("expected result[1] fail")
	}
	if !results[2].Passed {
		t.Error("expected result[2] pass")
	}
}

func TestParseSpecCoverageOutput_NoVerdictDefaultsFail(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "func Foo"}}
	results := parseSpecCoverageOutput("", assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected fail when no verdict returned")
	}
}

func TestParseSpecCoverageOutput_UsesDescription(t *testing.T) {
	assertions := []arc.GateAssertion{{Description: "custom desc", SpecCoverage: "func Foo"}}
	results := parseSpecCoverageOutput("PASS 1: func Foo — ok\n", assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Description != "custom desc" {
		t.Errorf("expected description %q, got %q", "custom desc", results[0].Description)
	}
}

func TestParseSpecCoverageOutput_DefaultDescription(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "MyFunc"}}
	results := parseSpecCoverageOutput("", assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Description == "" {
		t.Error("expected non-empty default description")
	}
}

func TestParseSpecCoverageOutput_OutOfRangeIndexIgnored(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "func Foo"}}
	// Index 99 is out of range — should be ignored, result defaults to fail.
	output := "PASS 99: func Whatever — ok\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("out-of-range verdict should be ignored, result should default to fail")
	}
}
