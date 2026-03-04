package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildGoTestCommandAbsolutePath verifies that an absolute path for TestFile
// doesn't produce an invalid Go package path like ".//abs/path/".
// The spec says package path should be "./" + dir(file) + "/" but for absolute
// paths, filepath.Dir returns an absolute dir, producing ".//tmp/foo/" which
// is not a valid Go import path.
func TestBuildGoTestCommandAbsolutePath(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "/tmp/some/dir/foo_test.go",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// The package arg should not contain "//" (double slash)
	for _, a := range args {
		if strings.Contains(a, "//") {
			t.Fatalf("absolute path produced invalid package with double slash: %v", args)
		}
	}
}

// TestBuildGoTestCommandDotDotPath verifies that ".." in the TestFile path
// doesn't produce an invalid Go package path.
func TestBuildGoTestCommandDotDotPath(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "../other/pkg/foo_test.go",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// "./" + "../other/pkg" + "/" = "./../other/pkg/" — go test should reject this
	// but builtin doesn't validate; check the path is at least cleaned
	for _, a := range args {
		if a == "./../other/pkg/" {
			// This is what it would produce — technically valid for go test
			// but non-canonical. filepath.Clean would normalize it.
			t.Logf("produced non-canonical path: %s (go test may reject)", a)
		}
	}
}

// TestBuildGoTestCommandRootFile verifies that a test file at the root level
// (e.g., "foo_test.go") produces "./" not "././"
func TestBuildGoTestCommandRootFile(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "foo_test.go",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// filepath.Dir("foo_test.go") = "." so pkg = "./" + "." + "/" = "././"
	// The correct package should be "./" not "././"
	for _, a := range args {
		if a == "././" {
			t.Fatalf("root-level test file produced '././' instead of './' as package path: %v", args)
		}
	}
}

// TestCustomTestCommandFilterAppendedAsRawArg verifies that when TestCommand
// is set and a filter is provided, the filter isn't just appended as a bare
// argument. For example, with TestCommand="go test" and Filter="TestFoo",
// the command should not become "go test file.go TestFoo" because go test
// would interpret "TestFoo" as a package name, not a test filter.
func TestCustomTestCommandFilterAppendedAsRawArg(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		TestCommand: "go test",
		TestFile:    "some_test.go",
		Filter:      "TestFoo",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// The current implementation appends the filter as a bare arg after the file:
	//   ["go", "test", "some_test.go", "TestFoo"]
	// This is wrong for go test because "TestFoo" is treated as a package.
	// The filter should be passed as -run TestFoo or similar.
	// Verify the filter is NOT a bare trailing argument.
	if len(args) >= 4 && args[len(args)-1] == "TestFoo" {
		// Check that it's not just appended without a flag
		if args[len(args)-2] != "-run" && args[len(args)-2] != "-k" && args[len(args)-2] != "--filter" {
			t.Fatalf("filter 'TestFoo' appended as bare arg without flag prefix — "+
				"command would be %q which treats TestFoo as a package name, not filter",
				strings.Join(args, " "))
		}
	}
}

// TestParseGoTestOutputSubtests verifies that subtests are counted correctly.
// Go's verbose output includes both parent and subtest pass/fail lines:
//   --- PASS: TestFoo/sub1 (0.00s)
//   --- PASS: TestFoo/sub2 (0.00s)
//   --- PASS: TestFoo (0.00s)
// The parser should not double-count the parent and subtests as separate tests
// (or at minimum, the Total should reflect the actual test count).
func TestParseGoTestOutputSubtests(t *testing.T) {
	stdout := `=== RUN   TestFoo
=== RUN   TestFoo/sub1
=== RUN   TestFoo/sub2
--- PASS: TestFoo/sub1 (0.00s)
--- PASS: TestFoo/sub2 (0.00s)
--- PASS: TestFoo (0.00s)
PASS
ok  	example.com/pkg	0.01s
`
	result := parseTestOutput("go-test", stdout, "", 0)
	// There is 1 top-level test with 2 subtests. After deduplication,
	// the parent TestFoo is removed (it's only passing because subtests passed),
	// leaving 2 subtests as the actual test count.
	if result.Total != 2 {
		t.Fatalf("expected Total=2 (subtests only, parent deduplicated), got %d", result.Total)
	}
}

// TestParseGoTestOutputSubtestFailure verifies that when a subtest fails,
// the parent test also shows as FAIL, and we don't double-count failures.
func TestParseGoTestOutputSubtestFailure(t *testing.T) {
	stdout := `=== RUN   TestFoo
=== RUN   TestFoo/sub1
=== RUN   TestFoo/sub2
--- PASS: TestFoo/sub1 (0.00s)
--- FAIL: TestFoo/sub2 (0.01s)
--- FAIL: TestFoo (0.01s)
FAIL
`
	result := parseTestOutput("go-test", stdout, "", 1)
	// Two --- FAIL lines: TestFoo/sub2 and TestFoo
	// But only sub2 actually failed — TestFoo is marked FAIL because a subtest failed.
	// The parser counts both as failures, so Failed=2. But conceptually only 1 test failed.
	if result.Failed != 1 {
		t.Logf("subtest failure double-counted: Failed=%d, FailedNames=%v",
			result.Failed, result.FailedNames)
		// This IS the bug: the parser double-counts subtest + parent failures
		if result.Failed == 2 {
			t.Fatalf("BUG: subtest failure double-counted — Failed=%d but only 1 actual failure "+
				"(TestFoo marked FAIL because subtest TestFoo/sub2 failed). FailedNames=%v",
				result.Failed, result.FailedNames)
		}
	}
}

// TestSummaryPassedGtTotalEdge tests Summary() when Passed+Failed < Total
// (e.g., skipped tests). The total in the summary should use r.Total.
func TestSummaryPassedGtTotalEdge(t *testing.T) {
	r := &TestResult{
		Total:    10,
		Passed:   5,
		Failed:   2,
		Duration: time.Second,
		FailedNames: []string{"TestA", "TestB"},
	}
	summary := r.Summary()
	// 3 tests are unaccounted (skipped). Summary should show 5/10 not 5/7.
	if !strings.Contains(summary, "5/10") {
		t.Fatalf("expected 5/10 (using r.Total) but got %q", summary)
	}
}

// TestSummaryPassedNonzeroTotalZero tests the edge case where Total=0 but
// Passed > 0. The "0 tests" path checks Total==0 && Failed==0.
// With Total=0, Passed=3, Failed=0, the guard triggers and prints "0 tests"
// even though 3 tests passed. This is a spec violation.
func TestSummaryPassedNonzeroTotalZero(t *testing.T) {
	r := &TestResult{
		Total:    0,
		Passed:   3,
		Failed:   0,
		Duration: time.Second,
	}
	summary := r.Summary()
	// Should NOT say "0 tests" when Passed > 0
	if strings.Contains(summary, "0 tests") {
		t.Fatalf("BUG: Summary says '0 tests' but Passed=%d: %q", r.Passed, summary)
	}
}

// TestRunBuiltinGoIntegrationFailingTest verifies that a failing test produces
// correct Failed count and FailedNames.
func TestRunBuiltinGoIntegrationFailingTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)

	testCode := `package testmod

import "testing"

func TestWillPass(t *testing.T) {}
func TestWillFail(t *testing.T) { t.Fatal("intentional failure") }
func TestAlsoPass(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "mixed_test.go"), []byte(testCode), 0644)

	result, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "mixed_test.go",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed != 2 {
		t.Errorf("expected Passed=2, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", result.Failed)
	}
	if len(result.FailedNames) != 1 || result.FailedNames[0] != "TestWillFail" {
		t.Errorf("expected FailedNames=[TestWillFail], got %v", result.FailedNames)
	}
	if result.Total != 3 {
		t.Errorf("expected Total=3, got %d", result.Total)
	}
}

// TestRunBuiltinGoIntegrationCompileError verifies that a compile error
// is properly reported (no PASS/FAIL lines, non-zero exit).
func TestRunBuiltinGoIntegrationCompileError(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)

	// Code that won't compile
	testCode := `package testmod

import "testing"

func TestBroken(t *testing.T) {
	undefinedFunction()
}
`
	os.WriteFile(filepath.Join(dir, "broken_test.go"), []byte(testCode), 0644)

	result, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "broken_test.go",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should report at least 1 failure for compile error
	if result.Failed < 1 {
		t.Fatalf("expected Failed >= 1 for compile error, got %d", result.Failed)
	}
	// Raw output should contain the error
	if !strings.Contains(result.RawOutput, "undefined") {
		t.Fatalf("expected RawOutput to contain 'undefined', got %q", result.RawOutput)
	}
}

// TestBuildGoTestCommandTimeoutFormat verifies the timeout flag format.
// The spec says `-timeout {timeout}` but time.Duration.String() produces
// values like "5m0s" which go test accepts.
func TestBuildGoTestCommandTimeoutFormat(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "foo_test.go",
	}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Find the -timeout value
	for i, a := range args {
		if a == "-timeout" && i+1 < len(args) {
			val := args[i+1]
			// Should be a valid Go duration string
			if _, err := time.ParseDuration(val); err != nil {
				t.Fatalf("timeout value %q is not a valid Go duration: %v", val, err)
			}
			return
		}
	}
	t.Fatal("no -timeout flag found in args")
}

// TestParseGoTestOutputPanicInTest verifies parsing when a test panics.
// Go test output for panics doesn't produce "--- FAIL:" for the specific test
// in some cases — it just shows FAIL with the panic trace.
func TestParseGoTestOutputPanicInTest(t *testing.T) {
	stdout := `=== RUN   TestPanicker
--- FAIL: TestPanicker (0.00s)
panic: oh no [recovered]
	panic: oh no

goroutine 1 [running]:
testing.tRunner.func1.2(...)
FAIL	example.com/pkg	0.01s
`
	result := parseTestOutput("go-test", stdout, "", 1)
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1 for panicking test, got %d", result.Failed)
	}
	if len(result.FailedNames) != 1 || result.FailedNames[0] != "TestPanicker" {
		t.Fatalf("expected FailedNames=[TestPanicker], got %v", result.FailedNames)
	}
}

// TestParseGoTestOutputDataRace verifies parsing when go test detects a data race.
// Race detector output appears in stderr with "DATA RACE" warning.
func TestParseGoTestOutputDataRace(t *testing.T) {
	stdout := `=== RUN   TestRacy
--- FAIL: TestRacy (0.01s)
FAIL
`
	stderr := `==================
WARNING: DATA RACE
Read at 0x00c0000a6060 by goroutine 8:
==================
Found 1 data race(s)
exit status 66
FAIL	example.com/pkg	0.02s
`
	result := parseTestOutput("go-test", stdout, stderr, 1)
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1 for data race test, got %d", result.Failed)
	}
	// The raw output should contain the race warning
	if !strings.Contains(result.RawOutput, "DATA RACE") {
		t.Fatalf("expected RawOutput to contain DATA RACE warning")
	}
}
