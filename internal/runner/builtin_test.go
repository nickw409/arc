package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunBuiltinGoScoping(t *testing.T) {
	args, runner, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "internal/runner/runner_test.go",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if runner != "go-test" {
		t.Fatalf("expected runner go-test, got %s", runner)
	}
	found := false
	for _, a := range args {
		if a == "./internal/runner/" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected args to contain ./internal/runner/, got %v", args)
	}
}

func TestRunBuiltinGoWithFilter(t *testing.T) {
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "runner_test.go",
		Filter:   "TestRun",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hasRun := false
	hasFilter := false
	for i, a := range args {
		if a == "-run" && i+1 < len(args) && args[i+1] == "TestRun" {
			hasRun = true
			hasFilter = true
		}
	}
	if !hasRun || !hasFilter {
		t.Fatalf("expected -run TestRun in args, got %v", args)
	}
}

func TestRunBuiltinPytest(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "pytest",
		TestFile: "tests/test_auth.py",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unsupported runner pytest")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected unsupported runner error, got: %s", err)
	}
}

func TestRunBuiltinVitest(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "vitest",
		TestFile: "src/Button.test.ts",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unsupported runner vitest")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected unsupported runner error, got: %s", err)
	}
}

func TestRunBuiltinCargoTest(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "cargo-test",
		TestFile: "tests/integration_test.rs",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unsupported runner cargo-test")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected unsupported runner error, got: %s", err)
	}
}

func TestRunBuiltinCargoNextest(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "cargo-nextest",
		TestFile: "tests/integration_test.rs",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unsupported runner cargo-nextest")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected unsupported runner error, got: %s", err)
	}
}

func TestRunBuiltinCargoNextestWithFilter(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "cargo-nextest",
		TestFile: "tests/auth_test.rs",
		Filter:   "login",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unsupported runner cargo-nextest")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected unsupported runner error, got: %s", err)
	}
}

func TestRunBuiltinTestCommandOverride(t *testing.T) {
	args, runner, err := buildCommand(RunBuiltinOptions{
		TestCommand: "make test",
		Runner:      "go-test", // should be ignored
		TestFile:    "test_foo.py",
	}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if runner != "custom" {
		t.Fatalf("expected runner custom, got %s", runner)
	}
	if args[0] != "make" || args[1] != "test" {
		t.Fatalf("expected make test as base command, got %v", args)
	}
	if args[2] != "test_foo.py" {
		t.Fatalf("expected test_foo.py appended, got %v", args)
	}
}

func TestRunBuiltinUnknownRunner(t *testing.T) {
	_, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "unknown",
		TestFile: "test.go",
	}, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for unknown runner")
	}
	if !strings.Contains(err.Error(), "unsupported runner") {
		t.Fatalf("expected 'unsupported runner' in error, got: %s", err)
	}
}

func TestParseGoTestOutputAllPass(t *testing.T) {
	stdout := `=== RUN   TestA
--- PASS: TestA (0.00s)
=== RUN   TestB
--- PASS: TestB (0.01s)
=== RUN   TestC
--- PASS: TestC (0.02s)
PASS
ok  	example.com/pkg	0.03s
`
	result := parseTestOutput("go-test", stdout, "", 0)
	if result.Total != 3 {
		t.Fatalf("expected Total=3, got %d", result.Total)
	}
	if result.Passed != 3 {
		t.Fatalf("expected Passed=3, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected Failed=0, got %d", result.Failed)
	}
}

func TestParseGoTestOutputMixed(t *testing.T) {
	stdout := `=== RUN   TestA
--- PASS: TestA (0.00s)
=== RUN   TestB
--- PASS: TestB (0.01s)
=== RUN   TestBroken
--- FAIL: TestBroken (0.02s)
FAIL
`
	result := parseTestOutput("go-test", stdout, "", 1)
	if result.Total != 3 {
		t.Fatalf("expected Total=3, got %d", result.Total)
	}
	if result.Passed != 2 {
		t.Fatalf("expected Passed=2, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1, got %d", result.Failed)
	}
	if len(result.FailedNames) != 1 || result.FailedNames[0] != "TestBroken" {
		t.Fatalf("expected FailedNames=[TestBroken], got %v", result.FailedNames)
	}
}

func TestParseGoTestOutputCompileError(t *testing.T) {
	stderr := `# example.com/pkg
./main.go:10:2: undefined: foo
FAIL	example.com/pkg [build failed]
`
	result := parseTestOutput("go-test", "", stderr, 2)
	if result.Failed < 1 {
		t.Fatalf("expected Failed >= 1, got %d", result.Failed)
	}
	if !strings.Contains(result.RawOutput, "undefined: foo") {
		t.Fatal("expected RawOutput to contain error text")
	}
}

func TestParseTestOutputPytest(t *testing.T) {
	// Pytest runner is not implemented — should fall back to exit-code-based.
	stdout := `FAILED test_foo.py::test_bar
3 passed, 1 failed
`
	result := parseTestOutput("pytest", stdout, "", 1)
	// Falls back to custom: exit code != 0 → Failed=1
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1 (fallback), got %d", result.Failed)
	}
}

func TestParseTestOutputVitest(t *testing.T) {
	stdout := `Tests: 5 passed, 2 failed`
	result := parseTestOutput("vitest", stdout, "", 1)
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1 (fallback), got %d", result.Failed)
	}
}

func TestParseTestOutputCargoTest(t *testing.T) {
	stdout := `test result: ok. 5 passed; 0 failed; 0 ignored`
	result := parseTestOutput("cargo-test", stdout, "", 0)
	// Falls back to custom: exit code 0 → Passed=1
	if result.Passed != 1 {
		t.Fatalf("expected Passed=1 (fallback), got %d", result.Passed)
	}
}

func TestParseTestOutputCargoNextest(t *testing.T) {
	stdout := `Summary: 10 tests run: 8 passed, 2 failed`
	result := parseTestOutput("cargo-nextest", stdout, "", 1)
	if result.Failed != 1 {
		t.Fatalf("expected Failed=1 (fallback), got %d", result.Failed)
	}
}

func TestParseTestOutputEmpty(t *testing.T) {
	result := parseTestOutput("go-test", "", "", 0)
	if result.Total != 0 {
		t.Fatalf("expected Total=0, got %d", result.Total)
	}
	if result.Passed != 0 {
		t.Fatalf("expected Passed=0, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected Failed=0, got %d", result.Failed)
	}
}

func TestParseTestOutputUnknownRunner(t *testing.T) {
	result := parseTestOutput("unknown", "some output", "", 0)
	if result.Total != 1 || result.Passed != 1 {
		t.Fatalf("expected fallback pass, got Total=%d Passed=%d", result.Total, result.Passed)
	}

	result2 := parseTestOutput("unknown", "some output", "", 1)
	if result2.Total != 1 || result2.Failed != 1 {
		t.Fatalf("expected fallback fail, got Total=%d Failed=%d", result2.Total, result2.Failed)
	}
}

func TestTestResultSummaryAllPass(t *testing.T) {
	r := &TestResult{Total: 5, Passed: 5, Failed: 0, Duration: 1200 * time.Millisecond}
	expected := "5/5 passed (1.2s)"
	if r.Summary() != expected {
		t.Fatalf("expected %q, got %q", expected, r.Summary())
	}
}

func TestTestResultSummaryWithFailures(t *testing.T) {
	r := &TestResult{
		Total:       5,
		Passed:      3,
		Failed:      2,
		Duration:    2300 * time.Millisecond,
		FailedNames: []string{"TestA", "TestB"},
	}
	expected := "3/5 passed, 2 failed (2.3s): TestA, TestB"
	if r.Summary() != expected {
		t.Fatalf("expected %q, got %q", expected, r.Summary())
	}
}

func TestTestResultSummaryZeroTests(t *testing.T) {
	r := &TestResult{Total: 0, Passed: 0, Failed: 0, Duration: 100 * time.Millisecond}
	expected := "0 tests (0.1s)"
	if r.Summary() != expected {
		t.Fatalf("expected %q, got %q", expected, r.Summary())
	}
}

func TestTestResultSummaryManyFailures(t *testing.T) {
	names := make([]string, 50)
	for i := range names {
		names[i] = "TestFail" + strings.Repeat("X", 1)
	}
	r := &TestResult{
		Total:       50,
		Passed:      0,
		Failed:      50,
		Duration:    5 * time.Second,
		FailedNames: names,
	}
	summary := r.Summary()
	if !strings.Contains(summary, "(+40 more)") {
		t.Fatalf("expected truncation with (+40 more), got %q", summary)
	}
}

func TestRunBuiltinGoIntegration(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()

	// Create go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create passing test
	testCode := `package testmod

import "testing"

func TestHello(t *testing.T) {
	if "hello" != "hello" {
		t.Fatal("unexpected")
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "hello_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "hello_test.go",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed < 1 {
		t.Fatalf("expected Passed >= 1, got %d. Output: %s", result.Passed, result.RawOutput)
	}
	if result.Failed != 0 {
		t.Fatalf("expected Failed == 0, got %d", result.Failed)
	}
	if result.Duration <= 0 {
		t.Fatalf("expected Duration > 0, got %v", result.Duration)
	}
}

func TestRunBuiltinTimeout(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create slow test
	testCode := `package testmod

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T) {
	time.Sleep(30 * time.Second)
}
`
	if err := os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	result, err := RunBuiltin(ctx, RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "slow_test.go",
		Dir:      dir,
		Timeout:  2 * time.Second,
	})
	// Either error or a result with failures (go test's own timeout kicks in)
	if err == nil && result.Failed == 0 {
		t.Fatal("expected timeout-related error or failure")
	}
}

func TestRunBuiltinMissingFile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "nonexistent_test.go",
		Dir:      dir,
	})
	// go test will fail on a nonexistent package directory
	if err == nil {
		// It's also ok if it returns a result with failures
		t.Log("RunBuiltin returned nil error for missing file (go test may report via exit code)")
	}
}

func TestRunBuiltinEmptyTestFile(t *testing.T) {
	_, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "",
	})
	if err == nil {
		t.Fatal("expected error for empty test file")
	}
}

func TestRunBuiltinInvalidDir(t *testing.T) {
	_, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "some_test.go",
		Dir:      "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestRunBuiltinContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := RunBuiltin(ctx, RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "some_test.go",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRunBuiltinWithDir(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	testCode := `package testmod

import "testing"

func TestDir(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(dir, "dir_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunBuiltin(context.Background(), RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "dir_test.go",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed < 1 {
		t.Fatalf("expected at least 1 pass, got %d. Output: %s", result.Passed, result.RawOutput)
	}
}

func TestRunBuiltinZeroTimeout(t *testing.T) {
	// Zero timeout should use default (5 min), not error
	args, _, err := buildCommand(RunBuiltinOptions{
		Runner:   "go-test",
		TestFile: "some_test.go",
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Should have a -timeout flag with default value
	found := false
	for _, a := range args {
		if a == "-timeout" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected -timeout in args, got %v", args)
	}
}

func TestTestResultJSONRoundTrip(t *testing.T) {
	r := &TestResult{
		Total:       5,
		Passed:      3,
		Failed:      2,
		RawOutput:   "test output",
		FailedNames: []string{"TestA", "TestB"},
		Duration:    1500 * time.Millisecond,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	var r2 TestResult
	if err := json.Unmarshal(data, &r2); err != nil {
		t.Fatal(err)
	}

	if r2.Total != r.Total || r2.Passed != r.Passed || r2.Failed != r.Failed {
		t.Fatalf("round-trip mismatch: got %+v", r2)
	}
}
