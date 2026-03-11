package gate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ---------------------------------------------------------------------------
// deriveFileExistsAssertions — edge cases
// ---------------------------------------------------------------------------

// Duplicate entries in spec.Files produce duplicate assertions — each copy of
// the file gets its own file_exists check. This documents the current behavior
// so that any future deduplication is a deliberate change.
func TestDeriveFileExistsAssertions_DuplicateFiles(t *testing.T) {
	assertions := deriveFileExistsAssertions([]string{"foo.go", "foo.go"}, nil)
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions for 2 identical paths, got %d", len(assertions))
	}
	if assertions[0].FileExists != "foo.go" || assertions[1].FileExists != "foo.go" {
		t.Errorf("unexpected assertion values: %+v", assertions)
	}
}

// An empty string in spec.Files is passed through and produces a file_exists
// assertion for path "". At runtime this checks whether workdir itself exists
// (filepath.Join(workdir, "") == workdir). Document this so it isn't a surprise.
func TestDeriveFileExistsAssertions_EmptyStringInFiles(t *testing.T) {
	assertions := deriveFileExistsAssertions([]string{""}, nil)
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion for empty-string path, got %d", len(assertions))
	}
	if assertions[0].FileExists != "" {
		t.Errorf("expected FileExists to be empty string, got %q", assertions[0].FileExists)
	}
}

// A file already covered by a legacy Type+Target assertion should not get a
// second derived file_exists assertion.
func TestDeriveFileExistsAssertions_CoveredByLegacyType(t *testing.T) {
	existing := []arc.GateAssertion{
		{Type: "file_exists", Target: "main.go"},
	}
	assertions := deriveFileExistsAssertions([]string{"main.go", "other.go"}, existing)
	// "main.go" is covered by the legacy assertion; "other.go" is not.
	if len(assertions) != 1 {
		t.Fatalf("expected 1 derived assertion (main.go already covered), got %d: %v", len(assertions), assertions)
	}
	if assertions[0].FileExists != "other.go" {
		t.Errorf("expected FileExists=other.go, got %q", assertions[0].FileExists)
	}
}

// A file covered by a direct FileExists field on an existing assertion should
// not get a second derived assertion.
func TestDeriveFileExistsAssertions_CoveredByDirectField(t *testing.T) {
	existing := []arc.GateAssertion{
		{FileExists: "main.go"},
	}
	assertions := deriveFileExistsAssertions([]string{"main.go"}, existing)
	if len(assertions) != 0 {
		t.Fatalf("expected 0 derived assertions (main.go already covered), got %d", len(assertions))
	}
}

// Empty files list always returns nil regardless of existing assertions.
func TestDeriveFileExistsAssertions_EmptyFilesNilResult(t *testing.T) {
	assertions := deriveFileExistsAssertions(nil, []arc.GateAssertion{{FileExists: "x.go"}})
	if assertions != nil {
		t.Errorf("expected nil for empty files list, got %v", assertions)
	}
}

// ---------------------------------------------------------------------------
// derivePromiseAssertions — whitespace inconsistency in TestCovers
// ---------------------------------------------------------------------------

// TestCovers does NOT apply TrimSpace to its own field before the switch check,
// unlike FuncExists/TestExists/FileExists. A whitespace-only TestCovers with a
// non-empty Test field therefore DOES match the switch case and creates an
// assertion. This test documents that inconsistency.
func TestDerivePromiseAssertions_WhitespaceOnlyTestCovers_StillMatches(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{
		{TestCovers: "   ", Test: "TestFoo"},
	})
	// The switch case `p.TestCovers != "" && p.Test != ""` matches because
	// "   " != "". This creates a test_exists assertion and adds "   " to
	// testCoversItems — the whitespace target leaks into the result.
	if len(assertions) != 1 {
		t.Errorf("expected 1 assertion (whitespace TestCovers passes the != '' check), got %d", len(assertions))
	}
	if len(items) != 1 {
		t.Errorf("expected 1 testCoversItem, got %d", len(items))
	}
	if len(items) > 0 && strings.TrimSpace(items[0]) == "" {
		// The leaked item is whitespace-only — document it.
		t.Logf("whitespace target leaked into TestCoversQueue: %q", items[0])
	}
}

// FuncExists whitespace-only is correctly rejected (uses TrimSpace in the check).
func TestDerivePromiseAssertions_FuncExists_WhitespaceOnly_Rejected(t *testing.T) {
	assertions, _ := derivePromiseAssertions([]arc.Promise{{FuncExists: "\t  \n"}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions for whitespace-only FuncExists, got %d", len(assertions))
	}
}

// ---------------------------------------------------------------------------
// isSuspicious — boundary cases not covered by existing tests
// ---------------------------------------------------------------------------

func TestIsSuspicious_TODO_Exact_IsMatch(t *testing.T) {
	if !isSuspicious("TODO") {
		t.Error("exact 'TODO' should be suspicious")
	}
}

func TestIsSuspicious_TODO_WithExtension_NotSuspicious(t *testing.T) {
	// "TODO.txt" has no suffix match (.txt != .tmp/.bak/.orig), no prefix match,
	// and is not the exact string "TODO". Should NOT be suspicious.
	if isSuspicious("TODO.txt") {
		t.Error("'TODO.txt' should NOT be suspicious — only exact 'TODO' is matched")
	}
}

func TestIsSuspicious_TODO_InSubpath_ChecksBaseName(t *testing.T) {
	// isSuspicious uses filepath.Base, so path/to/TODO is "TODO" at base level.
	if !isSuspicious("path/to/TODO") {
		t.Error("'path/to/TODO' base is 'TODO' — should be suspicious")
	}
}

func TestIsSuspicious_Scratch_AloneIsSuspicious(t *testing.T) {
	if !isSuspicious("scratch") {
		t.Error("'scratch' starts with 'scratch' prefix — should be suspicious")
	}
}

func TestIsSuspicious_Scratchpad_IsSuspicious(t *testing.T) {
	if !isSuspicious("scratchpad.go") {
		t.Error("'scratchpad.go' starts with 'scratch' — should be suspicious")
	}
}

func TestIsSuspicious_DebugAlone_IsSuspicious(t *testing.T) {
	// "debug_" starts with "debug_" prefix.
	if !isSuspicious("debug_") {
		t.Error("'debug_' should be suspicious (matches debug_ prefix)")
	}
}

func TestIsSuspicious_DebugNoUnderscore_NotSuspicious(t *testing.T) {
	// "debug" does NOT start with "debug_" (the underscore is part of the prefix pattern).
	if isSuspicious("debug") {
		t.Error("'debug' should NOT be suspicious — prefix is 'debug_' with underscore")
	}
}

// ---------------------------------------------------------------------------
// matchGlob — boundary cases
// ---------------------------------------------------------------------------

func TestMatchGlob_EmptyPattern_NothingMatches(t *testing.T) {
	// Empty pattern: filepath.Match("", "anything") returns false.
	if matchGlob("foo.go", "") {
		t.Error("empty pattern should not match 'foo.go'")
	}
}

func TestMatchGlob_BareSlashStarStar_Behavior(t *testing.T) {
	// Pattern "/**" → prefix = "". Rule: file == "" OR strings.HasPrefix(file, "/").
	// Relative paths (no leading slash) do NOT match, only absolute paths or "".
	if matchGlob("foo.go", "/**") {
		t.Error("relative path 'foo.go' should not match '/**' (no leading slash)")
	}
	if !matchGlob("/absolute/path.go", "/**") {
		t.Error("absolute path '/absolute/path.go' should match '/**'")
	}
}

func TestMatchGlob_PrefixExactMatch(t *testing.T) {
	// The file path exactly equals the prefix (without the /**).
	if !matchGlob("internal", "internal/**") {
		t.Error("file == prefix should match pattern 'prefix/**'")
	}
}

func TestMatchGlob_DeepNestedPath(t *testing.T) {
	if !matchGlob("a/b/c/d/e.go", "a/**") {
		t.Error("deeply nested path should match 'a/**'")
	}
}

func TestMatchGlob_PrefixSlashNotSubdir(t *testing.T) {
	// "internals/foo.go" should NOT match "internal/**" because the prefix
	// check uses HasPrefix(file, prefix+"/") — "internals/" does not start
	// with "internal/".
	if matchGlob("internals/foo.go", "internal/**") {
		t.Error("'internals/foo.go' should not match 'internal/**' (different prefix)")
	}
}

// ---------------------------------------------------------------------------
// RunSpecCoverage — edge cases (early-exit only; agent calls not unit-tested)
// ---------------------------------------------------------------------------

// workdir doesn't exist — collectTestFiles returns an error; RunSpecCoverage
// returns a single failed assertion without spawning an agent.
func TestRunSpecCoverage_WorkdirDoesNotExist(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "nonexistent")

	assertions := []arc.GateAssertion{
		{SpecCoverage: "SomeFunc"},
	}
	results := RunSpecCoverage(t.Context(), &arc.PhaseSpec{}, assertions, workdir)
	if len(results) != 1 {
		t.Fatalf("expected 1 error result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected fail when workdir does not exist")
	}
}

// ---------------------------------------------------------------------------
// parseSpecCoverageOutput — edge cases
// ---------------------------------------------------------------------------

// Mixed pass/fail output parsed correctly.
func TestParseSpecCoverageOutput_MixedPassFail(t *testing.T) {
	assertions := []arc.GateAssertion{
		{SpecCoverage: "TestPresent"},
		{SpecCoverage: "TestMissing"},
	}
	output := "PASS 1: TestPresent — found in foo_test.go\nFAIL 2: TestMissing — no test found\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected results[0] to pass")
	}
	if results[1].Passed {
		t.Errorf("expected results[1] to fail")
	}
}

// Lines without PASS/FAIL prefix are ignored.
func TestParseSpecCoverageOutput_IgnoresNonVerdictLines(t *testing.T) {
	assertions := []arc.GateAssertion{{SpecCoverage: "func Foo"}}
	output := "Here is my analysis:\nPASS 1: func Foo — found\nDone.\n"
	results := parseSpecCoverageOutput(output, assertions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Error("expected pass")
	}
}

// collectTestFiles skips hidden directories (e.g., .git).
func TestCollectTestFiles_SkipsHiddenDirs(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, ".hidden/secret_test.go", "package secret\n")
	writeFile(t, workdir, "real_test.go", "package main\n")

	files, err := collectTestFiles(workdir)
	if err != nil {
		t.Fatalf("collectTestFiles: %v", err)
	}
	for _, f := range files {
		if strings.Contains(f, ".hidden") {
			t.Errorf("hidden directory should be skipped, got: %s", f)
		}
	}
	if len(files) != 1 {
		t.Errorf("expected 1 test file (hidden excluded), got %d: %v", len(files), files)
	}
}

// ---------------------------------------------------------------------------
// gate.Run — edge cases
// ---------------------------------------------------------------------------

// When gate.Run is called with an already-cancelled context, checkpoint
// execution fails but file-system assertions (which are synchronous and don't
// use ctx) still return results. The gate should fail due to checkpoint failure.
func TestRun_CancelledContext_CheckpointFails(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package foo\n")

	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - file_exists: foo.go
checkpoints:
  - name: cp
    description: "sleep"
    test: "sleep 60"
`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := Run(ctx, spec, workdir)
	if err != nil {
		t.Fatalf("Run should not return error (non-nil means spec was nil), got: %v", err)
	}
	// Checkpoint should have failed due to cancelled context.
	if result.Passed {
		t.Error("expected gate to fail when checkpoint runs with cancelled context")
	}
	// The file_exists assertion should still have run and passed.
	fileAssertionPassed := false
	for _, a := range result.Assertions {
		if strings.Contains(a.Detail, "foo.go") && a.Passed {
			fileAssertionPassed = true
		}
	}
	if !fileAssertionPassed {
		t.Logf("assertions: %+v", result.Assertions)
		// Note: file_exists doesn't use context, so it should pass regardless.
	}
}

// Checkpoint with no test command is "not_run" and does not count as a pass or
// failure — the gate should still pass if there are no other failures.
func TestRun_CheckpointNoTest_NotRun_GatePasses(t *testing.T) {
	workdir := t.TempDir()
	spec := parseSpec(t, `
name: test
spec: "test"
checkpoints:
  - name: no-test
    description: "no command"
`)
	result, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true when only checkpoint has no test, got false")
	}
	if len(result.Checkpoints) != 1 || result.Checkpoints[0].Status != "not_run" {
		t.Errorf("expected checkpoint status=not_run, got: %+v", result.Checkpoints)
	}
}

// spec_coverage runs after regular assertions even when earlier assertions fail.
// The spec_coverage results should appear in the Assertions slice.
func TestRun_SpecCoverage_RunsAfterFailedRegularAssertions(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package foo\n\nfunc Foo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package foo\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\tif got := Foo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
name: test
spec: "implement foo"
gate:
  assertions:
    - file_exists: nonexistent.go
    - spec_coverage: TestFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Error("expected fail — nonexistent.go missing")
	}
	// spec_coverage should still have run and its result should appear.
	foundCovPass := false
	for _, a := range result.Assertions {
		if strings.Contains(a.Description, "spec_coverage") && a.Passed {
			foundCovPass = true
		}
	}
	if !foundCovPass {
		t.Errorf("spec_coverage assertion should still run and pass even when earlier assertion failed: %+v", result.Assertions)
	}
}

// Duplicate files in spec.Files creates duplicate file_exists assertions via Run.
func TestRun_DuplicateFiles_CreatesDuplicateAssertions(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package foo\n")

	spec := parseSpec(t, `
name: test
spec: "implement foo"
files:
  - foo.go
  - foo.go
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass — foo.go exists: %+v", result.Assertions)
	}
	// Both duplicated file_exists assertions should pass.
	if len(result.Assertions) != 2 {
		t.Errorf("expected 2 assertions (duplicated file), got %d", len(result.Assertions))
	}
}

// checkFileExists succeeds when the path points to a directory (os.Stat succeeds on dirs).
func TestRun_FileExists_Directory_Passes(t *testing.T) {
	workdir := t.TempDir()
	// Create a directory; file_exists should pass since Stat succeeds.
	if err := os.MkdirAll(filepath.Join(workdir, "mydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - file_exists: mydir
`)
	result, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected file_exists to pass for a directory path: %s", result.Assertions[0].Detail)
	}
}

// checkFileAbsent passes when the path is absent, and fails when it's a directory.
func TestRun_FileAbsent_DirectoryExists_Fails(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "mydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - file_absent: mydir
`)
	result, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected file_absent to fail when a directory exists at the path")
	}
}

// Unknown assertion type (no recognized field, no valid type+target) returns a
// failed assertion result with a descriptive detail, not a crash.
func TestRun_UnknownAssertionType_Fails(t *testing.T) {
	workdir := t.TempDir()
	// Build a spec with an assertion that has no recognized fields.
	spec := &arc.PhaseSpec{
		Spec: "test",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Description: "bad assertion", Type: "unknown_type", Target: "some_target"},
			},
		},
	}
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail for unknown assertion type")
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion result, got %d", len(result.Assertions))
	}
	if !strings.Contains(result.Assertions[0].Detail, "unknown assertion type") {
		t.Errorf("expected 'unknown assertion type' in detail, got: %q", result.Assertions[0].Detail)
	}
}

// gate.Run with only checkpoints (no spec, no assertions) should not fail-fast
// since checkpoints count as "content" via the len(spec.Checkpoints) check.
func TestRun_CheckpointsOnly_NoFailFast(t *testing.T) {
	workdir := t.TempDir()
	spec := parseSpec(t, `
name: test
checkpoints:
  - name: cp
    description: "passes"
    test: "true"
`)
	result, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected gate to pass (checkpoint passes), got fail")
	}
}

// ---------------------------------------------------------------------------
// Concurrent gate.Run calls — verify no shared mutable state / data races
// ---------------------------------------------------------------------------

func TestRun_ConcurrentCalls_NoRace(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package foo\n\nfunc Bar() {}\n")

	spec := parseSpec(t, `
name: concurrent
spec: "concurrent test"
gate:
  assertions:
    - file_exists: foo.go
    - grep: "func Bar"
`)

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
			if err != nil {
				errs <- "Run error: " + err.Error()
				return
			}
			if !result.Passed {
				errs <- "expected Passed=true in concurrent run"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

