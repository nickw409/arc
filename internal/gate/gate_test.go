package gate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseSpec parses a YAML string into *arc.PhaseSpec for use in gate.Run.
func parseSpec(t *testing.T, content string) *arc.PhaseSpec {
	t.Helper()
	var spec arc.PhaseSpec
	if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("parsing spec YAML: %v", err)
	}
	return &spec
}

// writeSpec writes YAML content to spec.yaml in dir and returns the path.
// Used only by HasAssertions tests which still read from spec.yaml.
func writeSpec(t *testing.T, dir, content string) string {
	t.Helper()
	specPath := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing spec.yaml: %v", err)
	}
	return specPath
}

// writeFile creates a file inside dir with the given content.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return full
}

// ---------------------------------------------------------------------------
// Run — file_exists assertions
// ---------------------------------------------------------------------------

func TestRun_FileExists_Pass(t *testing.T) {
	workdir := t.TempDir()

	// Create the target file.
	writeFile(t, workdir, "internal/api/auth.go", "package api\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "auth.go exists"
      file_exists: internal/api/auth.go
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(result.Assertions))
	}
	if !result.Assertions[0].Passed {
		t.Errorf("expected assertion to pass: %v", result.Assertions[0].Detail)
	}
}

func TestRun_FileExists_Fail(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "missing.go exists"
      file_exists: internal/missing.go
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(result.Assertions))
	}
	if result.Assertions[0].Passed {
		t.Errorf("expected assertion to fail")
	}
}

// ---------------------------------------------------------------------------
// Run — grep assertions
// ---------------------------------------------------------------------------

func TestRun_Grep_Pass(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "internal/api/auth.go", `package api

func NewMiddleware() {}
`)

	spec := `
name: test-phase
gate:
  assertions:
    - description: "NewMiddleware exists"
      grep: "func NewMiddleware"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
	if !result.Assertions[0].Passed {
		t.Errorf("expected grep assertion to pass: %v", result.Assertions[0].Detail)
	}
}

func TestRun_Grep_Fail(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "internal/api/auth.go", "package api\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "NewMiddleware exists"
      grep: "func NewMiddleware"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false")
	}
	if result.Assertions[0].Passed {
		t.Errorf("expected grep assertion to fail")
	}
}

// ---------------------------------------------------------------------------
// Run — test_exists assertions
// ---------------------------------------------------------------------------

func TestRun_TestExists_Pass(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "internal/api/auth_test.go", `package api

import "testing"

func TestTokenExpiry(t *testing.T) {}
`)

	spec := `
name: test-phase
gate:
  assertions:
    - description: "TestTokenExpiry exists"
      test_exists: TestTokenExpiry
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
	if !result.Assertions[0].Passed {
		t.Errorf("expected test_exists assertion to pass: %v", result.Assertions[0].Detail)
	}
}

func TestRun_TestExists_Fail(t *testing.T) {
	workdir := t.TempDir()

	// Write a test file that does NOT contain the searched function.
	writeFile(t, workdir, "internal/api/auth_test.go", `package api

import "testing"

func TestSomethingElse(t *testing.T) {}
`)

	spec := `
name: test-phase
gate:
  assertions:
    - description: "TestTokenExpiry exists"
      test_exists: TestTokenExpiry
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false")
	}
	if result.Assertions[0].Passed {
		t.Errorf("expected test_exists assertion to fail")
	}
}

// Verify that grep only searches .go files, not _test.go.
func TestRun_TestExists_DoesNotMatchRegularGoFiles(t *testing.T) {
	workdir := t.TempDir()

	// Function is in a regular .go file, NOT a _test.go file.
	writeFile(t, workdir, "pkg/foo.go", `package pkg

func TestLookalike() {}
`)

	spec := `
name: test-phase
gate:
  assertions:
    - description: "TestLookalike in test file"
      test_exists: TestLookalike
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	// Should fail because the function is not in a _test.go file.
	if result.Passed {
		t.Errorf("expected Passed=false (function is in non-test file)")
	}
}

// ---------------------------------------------------------------------------
// Run — build_passes assertions
// ---------------------------------------------------------------------------

func TestRun_BuildPasses_Pass(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "true exits 0"
      build_passes: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(result.Assertions))
	}
	if !result.Assertions[0].Passed {
		t.Errorf("expected assertion to pass: %v", result.Assertions[0].Detail)
	}
}

func TestRun_BuildPasses_Fail(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "false exits 1"
      build_passes: "false"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(result.Assertions))
	}
	if result.Assertions[0].Passed {
		t.Errorf("expected assertion to fail")
	}
	if result.Assertions[0].Detail == "" {
		t.Errorf("expected detail to contain failure info")
	}
}

func TestRun_BuildPasses_TypeTarget(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "build via type field"
      type: build_passes
      target: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_BuildPasses_OutputCaptured(t *testing.T) {
	workdir := t.TempDir()

	// A command that produces output and fails.
	spec := `
name: test-phase
gate:
  assertions:
    - description: "failing build with output"
      build_passes: "echo 'syntax error'; exit 1"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false")
	}
	if !strings.Contains(result.Assertions[0].Detail, "syntax error") {
		t.Errorf("expected output in detail, got: %s", result.Assertions[0].Detail)
	}
}

// ---------------------------------------------------------------------------
// Run — no_untracked assertions
// ---------------------------------------------------------------------------

func TestRun_NoUntracked_Pass_NoSuspicious(t *testing.T) {
	workdir := t.TempDir()

	// Initialize a git repo so git ls-files works.
	if out, err := runGit(t, workdir, "init"); err != nil {
		t.Skipf("git init failed (%v): %s", err, out)
	}
	// Create a normal tracked file (staged).
	writeFile(t, workdir, "main.go", "package main\n")
	runGit(t, workdir, "add", "main.go")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "no debug artifacts"
      no_untracked: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got: %v", result.Assertions[0].Detail)
	}
}

func TestRun_NoUntracked_Fail_TmpFile(t *testing.T) {
	workdir := t.TempDir()

	if out, err := runGit(t, workdir, "init"); err != nil {
		t.Skipf("git init failed (%v): %s", err, out)
	}
	// Drop a .tmp file (untracked, not gitignored).
	writeFile(t, workdir, "output.tmp", "temporary\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "no debug artifacts"
      no_untracked: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false when .tmp file is untracked")
	}
	if !strings.Contains(result.Assertions[0].Detail, "output.tmp") {
		t.Errorf("expected detail to mention output.tmp, got: %s", result.Assertions[0].Detail)
	}
}

func TestRun_NoUntracked_Fail_DebugPrefix(t *testing.T) {
	workdir := t.TempDir()

	if out, err := runGit(t, workdir, "init"); err != nil {
		t.Skipf("git init failed (%v): %s", err, out)
	}
	writeFile(t, workdir, "debug_output.go", "package main\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "no debug artifacts"
      no_untracked: "yes"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false for debug_ prefixed file")
	}
}

func TestRun_NoUntracked_Pass_NormalUntrackedFile(t *testing.T) {
	workdir := t.TempDir()

	if out, err := runGit(t, workdir, "init"); err != nil {
		t.Skipf("git init failed (%v): %s", err, out)
	}
	// An untracked file with a normal name should not trigger the assertion.
	writeFile(t, workdir, "newfeature.go", "package main\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "no debug artifacts"
      no_untracked: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true for non-suspicious untracked file, got: %s", result.Assertions[0].Detail)
	}
}

func TestRun_NoUntracked_TypeField(t *testing.T) {
	workdir := t.TempDir()

	if out, err := runGit(t, workdir, "init"); err != nil {
		t.Skipf("git init failed (%v): %s", err, out)
	}

	spec := `
name: test-phase
gate:
  assertions:
    - description: "no debug artifacts via type"
      type: no_untracked
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got: %v", result.Assertions[0].Detail)
	}
}

// isSuspicious_* tests exercise the helper directly.

func TestIsSuspicious_Tmp(t *testing.T) {
	if !isSuspicious("foo.tmp") {
		t.Error("expected foo.tmp to be suspicious")
	}
}

func TestIsSuspicious_Bak(t *testing.T) {
	if !isSuspicious("config.bak") {
		t.Error("expected config.bak to be suspicious")
	}
}

func TestIsSuspicious_Orig(t *testing.T) {
	if !isSuspicious("file.orig") {
		t.Error("expected file.orig to be suspicious")
	}
}

func TestIsSuspicious_DebugPrefix(t *testing.T) {
	if !isSuspicious("debug_output.go") {
		t.Error("expected debug_output.go to be suspicious")
	}
}

func TestIsSuspicious_ScratchPrefix(t *testing.T) {
	if !isSuspicious("scratch_notes.txt") {
		t.Error("expected scratch_notes.txt to be suspicious")
	}
	if !isSuspicious("scratch.go") {
		t.Error("expected scratch.go to be suspicious")
	}
}

func TestIsSuspicious_TODO(t *testing.T) {
	if !isSuspicious("TODO") {
		t.Error("expected TODO to be suspicious")
	}
}

func TestIsSuspicious_Normal(t *testing.T) {
	cases := []string{"main.go", "README.md", "config.yaml", "internal/api/auth.go"}
	for _, c := range cases {
		if isSuspicious(c) {
			t.Errorf("expected %q to NOT be suspicious", c)
		}
	}
}

// runGit is a test helper that runs a git command in dir.
func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Configure a fake identity so git doesn't fail on systems without global config.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ---------------------------------------------------------------------------
// Run — type-based assertions (legacy Type+Target approach)
// ---------------------------------------------------------------------------

func TestRun_TypeTarget_FileExists(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "myfile.go", "package main\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "myfile.go via type field"
      type: file_exists
      target: myfile.go
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_TypeTarget_Grep(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "main.go", "package main\n\nfunc main() {}\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "func main via type field"
      type: grep
      target: "func main"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_TypeTarget_TestExists(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "main_test.go", `package main

import "testing"

func TestFoo(t *testing.T) {}
`)

	spec := `
name: test-phase
gate:
  assertions:
    - description: "TestFoo via type field"
      type: test_exists
      target: TestFoo
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

// ---------------------------------------------------------------------------
// Run — missing spec file
// ---------------------------------------------------------------------------

func TestRun_NilSpec(t *testing.T) {
	workdir := t.TempDir()
	_, err := Run(context.Background(), nil, workdir)
	if err == nil {
		t.Fatal("expected error for nil spec, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — empty assertions (should pass)
// ---------------------------------------------------------------------------

func TestRun_EmptyAssertions_WithSpec_Pass(t *testing.T) {
	workdir := t.TempDir()

	// Empty assertions are OK if the spec has content (verifier can check it).
	spec := `
name: test-phase
spec: "implement the feature"
gate:
  assertions: []
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true when spec has content")
	}
}

// ---------------------------------------------------------------------------
// Run — scoped test command
// ---------------------------------------------------------------------------

func TestRun_ScopedTest_AlwaysSkipped(t *testing.T) {
	// Scoped test execution was removed — verify field is now acceptance criteria
	// for the verifier agent, not a shell command. Gate always marks scoped test
	// as skipped+passed regardless of verify content.
	workdir := t.TempDir()

	spec := `
name: test-phase
verify: "Ensure all API endpoints return 200"
gate:
  assertions: []
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
	if !result.ScopedTestPassed {
		t.Errorf("expected ScopedTestPassed=true (always true when skipped)")
	}
	if !result.ScopedTestSkipped {
		t.Errorf("expected ScopedTestSkipped=true")
	}
}

func TestRun_NoScopedTest_Skipped(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
spec: "placeholder spec"
gate:
  assertions: []
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.ScopedTestSkipped {
		t.Errorf("expected ScopedTestSkipped=true when no test command set")
	}
}

func TestRun_EmptySpec_Fails(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Error("expected gate to fail when spec has no assertions, no checkpoints, and no spec content")
	}
	if len(result.Assertions) == 0 || !strings.Contains(result.Assertions[0].Detail, "misconfigured") {
		t.Errorf("expected misconfigured error detail, got: %v", result.Assertions)
	}
}

// ---------------------------------------------------------------------------
// Run — checkpoint test commands
// ---------------------------------------------------------------------------

func TestRun_Checkpoint_Pass(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "Always passing"
    test: "true"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
	if len(result.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(result.Checkpoints))
	}
	if result.Checkpoints[0].Status != "pass" {
		t.Errorf("expected checkpoint status=pass, got %q", result.Checkpoints[0].Status)
	}
}

func TestRun_Checkpoint_Fail(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "Always failing"
    test: "false"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false when checkpoint fails")
	}
	if result.Checkpoints[0].Status != "fail" {
		t.Errorf("expected checkpoint status=fail, got %q", result.Checkpoints[0].Status)
	}
}

func TestRun_Checkpoint_NoTest_NotRun(t *testing.T) {
	workdir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "No test command"
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true when checkpoint has no test command")
	}
	if result.Checkpoints[0].Status != "not_run" {
		t.Errorf("expected checkpoint status=not_run, got %q", result.Checkpoints[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Run — idempotency
// ---------------------------------------------------------------------------

func TestRun_Idempotent(t *testing.T) {
	workdir := t.TempDir()

	writeFile(t, workdir, "pkg/foo.go", "package pkg\n\nfunc Bar() {}\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "foo.go exists"
      file_exists: pkg/foo.go
    - description: "Bar func"
      grep: "func Bar"
`
	parsedSpec := parseSpec(t, spec)

	r1, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	r2, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if r1.Passed != r2.Passed {
		t.Errorf("idempotency: Passed differs: %v vs %v", r1.Passed, r2.Passed)
	}
	for i := range r1.Assertions {
		if r1.Assertions[i].Passed != r2.Assertions[i].Passed {
			t.Errorf("idempotency: assertion[%d].Passed differs", i)
		}
	}
}

// ---------------------------------------------------------------------------
// WriteStatus / ReadStatus roundtrip
// ---------------------------------------------------------------------------

func TestWriteReadStatus_Roundtrip(t *testing.T) {
	phaseDir := t.TempDir()

	result := &arc.GateResult{
		Passed: true,
		Assertions: []arc.AssertionResult{
			{Description: "foo", Passed: true},
		},
		Checkpoints: []arc.CheckpointStatus{
			{Name: "cp1", Status: "pass"},
		},
		ScopedTestPassed:  true,
		ScopedTestSkipped: false,
	}

	if err := WriteStatus(phaseDir, result); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	status, err := ReadStatus(phaseDir)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}

	if status.Passed != result.Passed {
		t.Errorf("Passed: want %v, got %v", result.Passed, status.Passed)
	}
	if v, ok := status.Checkpoints["cp1"]; !ok || v != "pass" {
		t.Errorf("checkpoint cp1: want pass, got %q (ok=%v)", v, ok)
	}
	if status.LastRun == "" {
		t.Errorf("LastRun should not be empty")
	}
	if status.Checkpoints == nil {
		t.Errorf("Checkpoints map should not be nil")
	}
}

func TestReadStatus_Missing(t *testing.T) {
	phaseDir := t.TempDir()
	_, err := ReadStatus(phaseDir)
	if err == nil {
		t.Fatal("expected error reading missing gate-status.json, got nil")
	}
}

func TestWriteStatus_PreservesRunCount(t *testing.T) {
	phaseDir := t.TempDir()

	result := &arc.GateResult{Passed: false, Checkpoints: []arc.CheckpointStatus{}}

	if err := WriteStatus(phaseDir, result); err != nil {
		t.Fatalf("first WriteStatus: %v", err)
	}

	// Manually increment run count.
	if _, err := IncrementRunCount(phaseDir); err != nil {
		t.Fatalf("IncrementRunCount: %v", err)
	}

	s, err := ReadStatus(phaseDir)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	// After one WriteStatus (runCount=1) and one IncrementRunCount, count should be 2.
	if s.RunCount != 2 {
		t.Errorf("expected RunCount=2, got %d", s.RunCount)
	}
}

// ---------------------------------------------------------------------------
// IncrementRunCount
// ---------------------------------------------------------------------------

func TestIncrementRunCount_StartsAtOne(t *testing.T) {
	phaseDir := t.TempDir()

	count, err := IncrementRunCount(phaseDir)
	if err != nil {
		t.Fatalf("IncrementRunCount: %v", err)
	}
	if count != 1 {
		t.Errorf("expected initial count=1, got %d", count)
	}
}

func TestIncrementRunCount_Monotonic(t *testing.T) {
	phaseDir := t.TempDir()

	for i := 1; i <= 5; i++ {
		count, err := IncrementRunCount(phaseDir)
		if err != nil {
			t.Fatalf("IncrementRunCount iteration %d: %v", i, err)
		}
		if count != i {
			t.Errorf("iteration %d: expected count=%d, got %d", i, i, count)
		}
	}
}

// ---------------------------------------------------------------------------
// Format
// ---------------------------------------------------------------------------

func TestFormat_Pass(t *testing.T) {
	result := &arc.GateResult{
		Passed: true,
		Assertions: []arc.AssertionResult{
			{Description: "File internal/api/auth.go exists", Passed: true},
			{Description: "func NewMiddleware found", Passed: true},
		},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestPassed:  true,
		ScopedTestSkipped: false,
	}

	out := Format(result)

	if !strings.HasPrefix(out, "PASS\n") {
		t.Errorf("expected output to start with PASS, got:\n%s", out)
	}
	if strings.Contains(out, "[ ]") {
		t.Errorf("pass result should not contain failing checkmarks, got:\n%s", out)
	}
	if strings.Contains(out, "Fix the items") {
		t.Errorf("pass result should not contain failure footer, got:\n%s", out)
	}
}

func TestFormat_Fail(t *testing.T) {
	result := &arc.GateResult{
		Passed: false,
		Assertions: []arc.AssertionResult{
			{Description: "File internal/api/auth.go exists", Passed: true},
			{Description: "func NewMiddleware found", Passed: false, Detail: "pattern not found"},
			{Description: "TestTokenExpiry exists", Passed: false, Detail: "not found in any _test.go file"},
		},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestPassed:  true,
		ScopedTestSkipped: false,
	}

	out := Format(result)

	if !strings.HasPrefix(out, "FAIL\n") {
		t.Errorf("expected output to start with FAIL, got:\n%s", out)
	}
	if !strings.Contains(out, "[ ]") {
		t.Errorf("fail result should contain failing checkmarks, got:\n%s", out)
	}
	if !strings.Contains(out, "Fix the items above") {
		t.Errorf("fail result should contain failure footer, got:\n%s", out)
	}
}

func TestFormat_ScopedTestFailed(t *testing.T) {
	result := &arc.GateResult{
		Passed:            false,
		Assertions:        []arc.AssertionResult{},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestPassed:  false,
		ScopedTestSkipped: false,
		ScopedTestOutput:  "FAIL: some test failed\n",
	}

	out := Format(result)

	if !strings.Contains(out, "Scoped tests: FAILED") {
		t.Errorf("expected 'Scoped tests: FAILED' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: some test failed") {
		t.Errorf("expected scoped test output in format result, got:\n%s", out)
	}
}

func TestFormat_ScopedTestSkipped(t *testing.T) {
	result := &arc.GateResult{
		Passed:            true,
		Assertions:        []arc.AssertionResult{},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestPassed:  true,
		ScopedTestSkipped: true,
	}

	out := Format(result)

	// Scoped test line should not appear.
	if strings.Contains(out, "Scoped tests") {
		t.Errorf("skipped scoped test should not appear in output, got:\n%s", out)
	}
}

func TestFormat_CheckpointItems(t *testing.T) {
	result := &arc.GateResult{
		Passed:     false,
		Assertions: []arc.AssertionResult{},
		Checkpoints: []arc.CheckpointStatus{
			{Name: "cp-pass", Status: "pass"},
			{Name: "cp-fail", Status: "fail"},
			{Name: "cp-norun", Status: "not_run"},
		},
		ScopedTestSkipped: true,
	}

	out := Format(result)

	if !strings.Contains(out, `"cp-pass": pass`) {
		t.Errorf("expected cp-pass line, got:\n%s", out)
	}
	if !strings.Contains(out, `"cp-fail": fail`) {
		t.Errorf("expected cp-fail line, got:\n%s", out)
	}
	if !strings.Contains(out, `"cp-norun"`) {
		t.Errorf("expected cp-norun line, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Loop detection warning
// ---------------------------------------------------------------------------

func TestFormat_LoopDetection_Below_Threshold(t *testing.T) {
	result := &arc.GateResult{
		Passed:            false,
		Assertions:        []arc.AssertionResult{},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestSkipped: true,
	}

	out := FormatWithRunCount(result, 5)
	if strings.Contains(out, "WARNING") {
		t.Errorf("should not have loop warning at runCount=5, got:\n%s", out)
	}
}

func TestFormat_LoopDetection_At_Threshold(t *testing.T) {
	result := &arc.GateResult{
		Passed:            false,
		Assertions:        []arc.AssertionResult{},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestSkipped: true,
	}

	out := FormatWithRunCount(result, loopDetectionThreshold)
	if strings.Contains(out, "WARNING") {
		t.Errorf("should not have loop warning at exactly threshold (%d), got:\n%s", loopDetectionThreshold, out)
	}
}

func TestFormat_LoopDetection_Above_Threshold(t *testing.T) {
	result := &arc.GateResult{
		Passed:            false,
		Assertions:        []arc.AssertionResult{},
		Checkpoints:       []arc.CheckpointStatus{},
		ScopedTestSkipped: true,
	}

	out := FormatWithRunCount(result, loopDetectionThreshold+1)
	if !strings.Contains(out, "WARNING") {
		t.Errorf("expected loop detection warning at runCount=%d, got:\n%s", loopDetectionThreshold+1, out)
	}
	if !strings.Contains(out, "reconsider your approach") {
		t.Errorf("expected 'reconsider your approach' in warning, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Full integration: real spec.yaml + workdir
// ---------------------------------------------------------------------------

func TestRun_Integration_MultipleAssertions(t *testing.T) {
	workdir := t.TempDir()

	// Set up files.
	writeFile(t, workdir, "internal/api/auth.go", `package api

func NewMiddleware() {}
func Helper() {}
`)
	writeFile(t, workdir, "internal/api/auth_test.go", `package api

import "testing"

func TestTokenExpiry(t *testing.T) {}
func TestNewMiddleware(t *testing.T) {}
`)

	spec := `
name: auth-phase
description: "Auth middleware implementation"
gate:
  assertions:
    - description: "auth.go exists"
      file_exists: internal/api/auth.go
    - description: "NewMiddleware function"
      grep: "func NewMiddleware"
    - description: "TestTokenExpiry test"
      test_exists: TestTokenExpiry
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		for _, a := range result.Assertions {
			t.Logf("assertion %q: passed=%v detail=%s", a.Description, a.Passed, a.Detail)
		}
		t.Errorf("expected all assertions to pass")
	}
}

func TestRun_Integration_MixedResults(t *testing.T) {
	workdir := t.TempDir()

	// Only create some of the expected artifacts.
	writeFile(t, workdir, "internal/api/auth.go", "package api\n")
	// No NewMiddleware, no test file.

	spec := `
name: auth-phase
gate:
  assertions:
    - description: "auth.go exists"
      file_exists: internal/api/auth.go
    - description: "NewMiddleware function"
      grep: "func NewMiddleware"
    - description: "TestTokenExpiry test"
      test_exists: TestTokenExpiry
`
	parsedSpec := parseSpec(t, spec)

	result, err := Run(context.Background(), parsedSpec, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false with missing artifacts")
	}

	// First assertion should pass (file exists).
	if !result.Assertions[0].Passed {
		t.Errorf("assertion[0] (file_exists) should pass")
	}
	// Second and third should fail.
	if result.Assertions[1].Passed {
		t.Errorf("assertion[1] (grep) should fail — NewMiddleware not implemented")
	}
	if result.Assertions[2].Passed {
		t.Errorf("assertion[2] (test_exists) should fail — no test file")
	}

	// Verify Format output matches expected structure.
	out := Format(result)
	if !strings.HasPrefix(out, "FAIL\n") {
		t.Errorf("expected FAIL prefix, got:\n%s", out)
	}
	if !strings.Contains(out, "[x] auth.go exists") {
		t.Errorf("expected passing assertion in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[ ] NewMiddleware function") {
		t.Errorf("expected failing assertion in output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// HasAssertions
// ---------------------------------------------------------------------------

func TestHasAssertions_WithAssertions(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: test-phase
gate:
  assertions:
    - description: "file exists"
      file_exists: main.go
`
	specPath := writeSpec(t, dir, spec)

	has, err := HasAssertions(specPath)
	if err != nil {
		t.Fatalf("HasAssertions: %v", err)
	}
	if !has {
		t.Error("expected HasAssertions=true when assertions are defined")
	}
}

func TestHasAssertions_WithCheckpointTests(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "A checkpoint"
    test: "go test ./..."
`
	specPath := writeSpec(t, dir, spec)

	has, err := HasAssertions(specPath)
	if err != nil {
		t.Fatalf("HasAssertions: %v", err)
	}
	if !has {
		t.Error("expected HasAssertions=true when checkpoints have test commands")
	}
}

func TestHasAssertions_Empty(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: test-phase
gate:
  assertions: []
`
	specPath := writeSpec(t, dir, spec)

	has, err := HasAssertions(specPath)
	if err != nil {
		t.Fatalf("HasAssertions: %v", err)
	}
	if has {
		t.Error("expected HasAssertions=false when no assertions or checkpoint tests")
	}
}

func TestHasAssertions_CheckpointsWithoutTests(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "No test command"
`
	specPath := writeSpec(t, dir, spec)

	has, err := HasAssertions(specPath)
	if err != nil {
		t.Fatalf("HasAssertions: %v", err)
	}
	if has {
		t.Error("expected HasAssertions=false when checkpoints have no test commands")
	}
}

func TestShouldRunVerifier(t *testing.T) {
	tr := true
	fa := false
	tests := []struct {
		name           string
		override       *bool
		configVerifier string
		specVerifier   *bool
		complexity     string
		want           bool
	}{
		{"override true wins", &tr, "never", &fa, "simple", true},
		{"override false wins", &fa, "always", &tr, "complex", false},
		{"config always", nil, "always", nil, "simple", true},
		{"config never", nil, "never", &tr, "complex", false},
		{"auto + complex", nil, "auto", nil, "complex", true},
		{"auto + medium", nil, "auto", nil, "medium", true},
		{"auto + simple", nil, "auto", nil, "simple", false},
		{"auto + spec true", nil, "auto", &tr, "simple", true},
		{"auto + spec false", nil, "auto", &fa, "medium", false},
		{"auto + empty complexity", nil, "auto", nil, "", false},
		{"empty config + complex", nil, "", nil, "complex", true},
		{"empty config + simple", nil, "", nil, "simple", false},
		{"spec false overrides medium", nil, "", &fa, "medium", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRunVerifier(tt.override, tt.configVerifier, tt.specVerifier, tt.complexity)
			if got != tt.want {
				t.Errorf("ShouldRunVerifier(%v, %q, %v, %q) = %v, want %v",
					tt.override, tt.configVerifier, tt.specVerifier, tt.complexity, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// file_absent
// ---------------------------------------------------------------------------

func TestRun_FileAbsent_Pass(t *testing.T) {
	workdir := t.TempDir()
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - file_absent: docs/WORKFLOW_SCHEMA.md
`)
	res, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Errorf("expected pass when file absent, got fail: %s", res.Assertions[0].Detail)
	}
}

func TestRun_FileAbsent_Fail(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "docs/WORKFLOW_SCHEMA.md", "content")
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - file_absent: docs/WORKFLOW_SCHEMA.md
`)
	res, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Error("expected fail when file exists, got pass")
	}
	if !strings.Contains(res.Assertions[0].Detail, "WORKFLOW_SCHEMA.md") {
		t.Errorf("detail should mention file name, got: %s", res.Assertions[0].Detail)
	}
}

// ---------------------------------------------------------------------------
// grep_not
// ---------------------------------------------------------------------------

func TestRun_GrepNot_Pass(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "pkg/foo.go", "package pkg\n\nfunc Good() {}\n")
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - grep_not: "OldBadPattern"
`)
	res, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Errorf("expected pass when pattern absent, got fail: %s", res.Assertions[0].Detail)
	}
}

func TestRun_GrepNot_Fail(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "pkg/foo.go", "package pkg\n\nfunc OldBadPattern() {}\n")
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - grep_not: "OldBadPattern"
`)
	res, err := Run(context.Background(), spec, workdir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Error("expected fail when pattern present, got pass")
	}
	if !strings.Contains(res.Assertions[0].Detail, "OldBadPattern") {
		t.Errorf("detail should mention pattern, got: %s", res.Assertions[0].Detail)
	}
}

// ---------------------------------------------------------------------------
// no_modified
// ---------------------------------------------------------------------------

func TestRun_NoModified_Pass(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - no_modified: .gitkeep
`)
	res, err := Run(context.Background(), spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Errorf("expected pass for unmodified file, got fail: %s", res.Assertions[0].Detail)
	}
}

func TestRun_NoModified_Fail(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - no_modified: .gitkeep
`)
	res, err := Run(context.Background(), spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Error("expected fail for modified file, got pass")
	}
	if !strings.Contains(res.Assertions[0].Detail, ".gitkeep") {
		t.Errorf("detail should mention file, got: %s", res.Assertions[0].Detail)
	}
}

// ---------------------------------------------------------------------------
// files_only
// ---------------------------------------------------------------------------

func TestRun_FilesOnly_Pass(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Add a file under docs/ and modify it.
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "README.md"), []byte("new doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.CombinedOutput()
	}
	run("add", "docs/README.md")
	run("commit", "-m", "add doc")
	// Now modify the doc — it should be in diff HEAD~1..HEAD.
	// Actually for files_only we need uncommitted changes: modify after last commit.
	if err := os.WriteFile(filepath.Join(dir, "docs", "README.md"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - files_only: "docs/**"
`)
	res, err := Run(context.Background(), spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Errorf("expected pass: only docs/ modified, got fail: %s", res.Assertions[0].Detail)
	}
}

func TestRun_FilesOnly_Fail(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Modify the tracked .gitkeep (outside docs/).
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - files_only: "docs/**"
`)
	res, err := Run(context.Background(), spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Error("expected fail: file outside docs/ was modified")
	}
	if !strings.Contains(res.Assertions[0].Detail, ".gitkeep") {
		t.Errorf("detail should name the violating file, got: %s", res.Assertions[0].Detail)
	}
}

func TestRun_FilesOnly_MultiplePatterns(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Modify .gitkeep — allowed by "*.md, .gitkeep" but not by "docs/**" alone.
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := parseSpec(t, `
name: test
spec: "test"
gate:
  assertions:
    - files_only: "docs/**, .gitkeep"
`)
	res, err := Run(context.Background(), spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Errorf("expected pass with multi-pattern allowlist, got fail: %s", res.Assertions[0].Detail)
	}
}

// ---------------------------------------------------------------------------
// matchGlob unit tests
// ---------------------------------------------------------------------------

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		file    string
		pattern string
		want    bool
	}{
		{"docs/README.md", "docs/**", true},
		{"docs/sub/deep.md", "docs/**", true},
		{"docs", "docs/**", true},
		{"internal/foo.go", "docs/**", false},
		{"README.md", "*.md", true},
		{"internal/foo.go", "*.md", false},
		{".gitkeep", ".gitkeep", true},
		{"other", ".gitkeep", false},
	}
	for _, tc := range cases {
		got := matchGlob(tc.file, tc.pattern)
		if got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.file, tc.pattern, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// derivePromiseAssertions tests
// ---------------------------------------------------------------------------

func TestDerivePromiseAssertions_Empty(t *testing.T) {
	assertions, items := derivePromiseAssertions(nil)
	if assertions != nil {
		t.Errorf("expected nil assertions, got %v", assertions)
	}
	if items != nil {
		t.Errorf("expected nil testCoversItems, got %v", items)
	}
}

func TestDerivePromiseAssertions_EmptySlice(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions, got %d", len(assertions))
	}
	if len(items) != 0 {
		t.Errorf("expected 0 testCoversItems, got %d", len(items))
	}
}

func TestDerivePromiseAssertions_FuncExists(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{FuncExists: "func NewFoo()"}})
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	if assertions[0].Grep != "func NewFoo()" {
		t.Errorf("Grep = %q, want %q", assertions[0].Grep, "func NewFoo()")
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems, got %v", items)
	}
}

func TestDerivePromiseAssertions_FuncExists_EmptyString(t *testing.T) {
	assertions, _ := derivePromiseAssertions([]arc.Promise{{FuncExists: ""}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions for empty FuncExists, got %d", len(assertions))
	}
}

func TestDerivePromiseAssertions_TestExists(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{TestExists: "TestNewFoo"}})
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	if assertions[0].TestExists != "TestNewFoo" {
		t.Errorf("TestExists = %q, want %q", assertions[0].TestExists, "TestNewFoo")
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems")
	}
}

func TestDerivePromiseAssertions_TestExists_WhitespaceOnly(t *testing.T) {
	assertions, _ := derivePromiseAssertions([]arc.Promise{{TestExists: "   "}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions for whitespace-only TestExists, got %d", len(assertions))
	}
}

func TestDerivePromiseAssertions_FileExists(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{FileExists: "foo.go"}})
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	if assertions[0].FileExists != "foo.go" {
		t.Errorf("FileExists = %q, want %q", assertions[0].FileExists, "foo.go")
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems")
	}
}

func TestDerivePromiseAssertions_FileExists_WhitespaceOnly(t *testing.T) {
	assertions, _ := derivePromiseAssertions([]arc.Promise{{FileExists: "  "}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions for whitespace-only FileExists, got %d", len(assertions))
	}
}

func TestDerivePromiseAssertions_TestCovers(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{TestCovers: "NewFoo", Test: "TestNewFoo"}})
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	if assertions[0].TestExists != "TestNewFoo" {
		t.Errorf("TestExists = %q, want TestNewFoo", assertions[0].TestExists)
	}
	if len(items) != 1 || items[0] != "NewFoo" {
		t.Errorf("testCoversItems = %v, want [NewFoo]", items)
	}
}

func TestDerivePromiseAssertions_TestCovers_MissingTest(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{TestCovers: "NewFoo"}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions when Test is missing, got %d", len(assertions))
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems")
	}
}

func TestDerivePromiseAssertions_TestCovers_EmptyTest(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{TestCovers: "Foo", Test: ""}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions when Test is empty, got %d", len(assertions))
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems")
	}
}

func TestDerivePromiseAssertions_MultipleTypes(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{
		{FuncExists: "func A()"},
		{TestExists: "TestA"},
		{FileExists: "a.go"},
		{TestCovers: "A", Test: "TestA"},
	})
	if len(assertions) != 4 {
		t.Fatalf("expected 4 assertions, got %d", len(assertions))
	}
	if len(items) != 1 || items[0] != "A" {
		t.Errorf("testCoversItems = %v, want [A]", items)
	}
}

func TestDerivePromiseAssertions_MultipleTestCovers(t *testing.T) {
	_, items := derivePromiseAssertions([]arc.Promise{
		{TestCovers: "A", Test: "TestA"},
		{TestCovers: "B", Test: "TestB"},
	})
	if len(items) != 2 {
		t.Fatalf("expected 2 testCoversItems, got %d: %v", len(items), items)
	}
}

func TestDerivePromiseAssertions_MultipleFieldsSet(t *testing.T) {
	// When both func_exists and test_exists are set, only first switch case triggers.
	assertions, _ := derivePromiseAssertions([]arc.Promise{{FuncExists: "func A()", TestExists: "TestA"}})
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion (first match wins), got %d", len(assertions))
	}
	if assertions[0].Grep != "func A()" {
		t.Errorf("expected Grep assertion for FuncExists, got %+v", assertions[0])
	}
}

func TestDerivePromiseAssertions_AllFieldsEmpty(t *testing.T) {
	assertions, items := derivePromiseAssertions([]arc.Promise{{}})
	if len(assertions) != 0 {
		t.Errorf("expected 0 assertions for empty promise, got %d", len(assertions))
	}
	if len(items) != 0 {
		t.Errorf("expected no testCoversItems")
	}
}

// ---------------------------------------------------------------------------
// gate.Run — promise integration tests
// ---------------------------------------------------------------------------

func TestRun_Promise_FuncExists_Pass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n\nfunc NewFoo() *Foo { return nil }\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - func_exists: "func NewFoo()"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got fail: %+v", result.Assertions)
	}
}

func TestRun_Promise_FuncExists_Fail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - func_exists: "func NewFoo()"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail, got pass")
	}
}

func TestRun_Promise_FuncExists_SpecialChars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n\nfunc NewFoo(x int) *Foo { return nil }\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - func_exists: "func NewFoo(x int) *Foo"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass with special chars")
	}
}

func TestRun_Promise_TestExists_Pass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo_test.go", "package foo\n\nfunc TestNewFoo(t *testing.T) {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - test_exists: TestNewFoo
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass: %+v", result.Assertions)
	}
}

func TestRun_Promise_TestExists_Fail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo_test.go", "package foo\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - test_exists: TestMissing
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail")
	}
}

func TestRun_Promise_FileExists_Pass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - file_exists: foo.go
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass: %+v", result.Assertions)
	}
}

func TestRun_Promise_FileExists_Fail(t *testing.T) {
	dir := t.TempDir()
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - file_exists: missing.go
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail")
	}
}

func TestRun_Promise_TestCovers_Derives_TestExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo_test.go", "package foo\n\nfunc TestNewFoo(t *testing.T) {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - test_covers: NewFoo
    test: TestNewFoo
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass: %+v", result.Assertions)
	}
}

func TestRun_Promise_TestCovers_Fail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo_test.go", "package foo\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - test_covers: NewFoo
    test: TestNewFoo
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail")
	}
}

func TestRun_Promise_TestCovers_PopulatesGateResult(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo_test.go", "package foo\n\nfunc TestNewFoo(t *testing.T) {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - test_covers: NewFoo
    test: TestNewFoo
  - test_covers: Bar
    test: TestBar
`)
	writeFile(t, dir, "bar_test.go", "package foo\n\nfunc TestBar(t *testing.T) {}\n")
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.TestCoversQueue) != 2 {
		t.Fatalf("TestCoversQueue = %v, want 2 items", result.TestCoversQueue)
	}
}

func TestRun_PromisesAndExplicitAssertions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n\nfunc NewFoo() {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
gate:
  assertions:
    - file_exists: foo.go
promises:
  - func_exists: "func NewFoo()"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass: %+v", result.Assertions)
	}
	if len(result.Assertions) != 2 {
		t.Errorf("expected 2 assertions (1 explicit + 1 promise), got %d", len(result.Assertions))
	}
}

func TestRun_PromisesAndFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n\nfunc NewFoo() {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
files:
  - foo.go
promises:
  - func_exists: "func NewFoo()"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass: %+v", result.Assertions)
	}
}

func TestRun_OnlyPromises_NoFailFast(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package foo\n\nfunc NewFoo() {}\n")
	spec := parseSpec(t, `
name: p
spec: implement foo
promises:
  - func_exists: "func NewFoo()"
`)
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass for promises-only spec")
	}
}

func TestRun_EmptySpec_FailFast(t *testing.T) {
	dir := t.TempDir()
	spec := &arc.PhaseSpec{Name: "p"}
	result, err := Run(context.Background(), spec, dir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail for empty spec (no assertions, no promises, no content)")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — Files + Promises + spec_coverage combined
// ---------------------------------------------------------------------------

func TestRun_FilesAndPromises_Combined_Pass(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() {}\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestNewFoo(t *testing.T) {}\n")

	spec := parseSpec(t, `
spec: "implement NewFoo"
complexity: simple
files:
  - foo.go
promises:
  - func_exists: "func NewFoo"
  - test_exists: TestNewFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result.Assertions)
	}
	// 1 file_exists (from Files) + 1 grep (FuncExists promise) + 1 test_exists (TestExists promise)
	if len(result.Assertions) != 3 {
		t.Fatalf("expected 3 assertions, got %d: %+v", len(result.Assertions), result.Assertions)
	}
}

func TestRun_FilesAndPromises_PartialFail(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() {}\n")
	// bar.go intentionally missing

	spec := parseSpec(t, `
spec: "implement NewFoo and bar"
complexity: simple
files:
  - foo.go
  - bar.go
promises:
  - func_exists: "func NewFoo"
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Passed {
		t.Fatalf("expected Passed=false (bar.go missing), got true")
	}
	// foo.go exists (pass), bar.go missing (fail), func_exists (pass)
	if len(result.Assertions) != 3 {
		t.Fatalf("expected 3 assertions, got %d: %+v", len(result.Assertions), result.Assertions)
	}
	if !result.Assertions[0].Passed {
		t.Errorf("expected assertions[0] (foo.go) to pass, got: %s", result.Assertions[0].Detail)
	}
	if result.Assertions[1].Passed {
		t.Errorf("expected assertions[1] (bar.go) to fail")
	}
	if !result.Assertions[2].Passed {
		t.Errorf("expected assertions[2] (func_exists) to pass, got: %s", result.Assertions[2].Detail)
	}
}

func TestRun_NoExplicitAssertions_FilesAndPromisesDriveGate(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "example.go", "package main\n\nfunc Example() {}\n")

	spec := parseSpec(t, `
spec: "implement Example"
complexity: simple
files:
  - example.go
promises:
  - func_exists: "func Example"
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected gate to pass with auto-derived assertions, got false: %+v", result.Assertions)
	}
	if len(result.Assertions) == 0 {
		t.Fatal("expected auto-derived assertions, got none")
	}
}

func TestRun_FilesOnly_AllMissing(t *testing.T) {
	workdir := t.TempDir()
	// No files created

	spec := parseSpec(t, `
spec: "create missing files"
complexity: simple
files:
  - missing1.go
  - missing2.go
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected Passed=false (all files missing)")
	}
	if len(result.Assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(result.Assertions))
	}
	for i, a := range result.Assertions {
		if a.Passed {
			t.Errorf("expected assertions[%d] to fail (file missing)", i)
		}
	}
}

func TestRun_PromisesOnly_AllFailing(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc Unrelated() {}\n")

	spec := parseSpec(t, `
spec: "implement missing functions"
complexity: simple
promises:
  - func_exists: "func Missing1"
  - test_exists: TestMissing2
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected Passed=false (promises not met)")
	}
	for i, a := range result.Assertions {
		if a.Passed {
			t.Errorf("expected assertions[%d] to fail", i)
		}
	}
}

func TestRun_SpecCoverageOnly_Pass(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc Foo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\tif got := Foo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
spec: "implement Foo"
complexity: simple
gate:
  assertions:
    - spec_coverage: TestFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result.Assertions)
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion (spec_coverage), got %d", len(result.Assertions))
	}
}

func TestRun_FilesAndSpecCoverage_Combined(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestNewFoo(t *testing.T) {\n\tif got := NewFoo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
spec: "implement NewFoo"
complexity: simple
files:
  - foo.go
gate:
  assertions:
    - spec_coverage: TestNewFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result.Assertions)
	}
	// 1 file_exists (from Files) + 1 spec_coverage
	if len(result.Assertions) < 2 {
		t.Fatalf("expected at least 2 assertions (file_exists + spec_coverage), got %d", len(result.Assertions))
	}
}

func TestRun_PromisesAndSpecCoverage_Combined(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestNewFoo(t *testing.T) {\n\tif got := NewFoo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
spec: "implement NewFoo"
complexity: simple
promises:
  - func_exists: "func NewFoo"
gate:
  assertions:
    - spec_coverage: TestNewFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result.Assertions)
	}
	// 1 grep (from promise) + 1 spec_coverage
	if len(result.Assertions) < 2 {
		t.Fatalf("expected at least 2 assertions (func_exists + spec_coverage), got %d", len(result.Assertions))
	}
}

func TestRun_AllThreeFeatures_Combined_Pass(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestNewFoo(t *testing.T) {\n\tif got := NewFoo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
spec: "implement NewFoo"
complexity: simple
files:
  - foo.go
promises:
  - func_exists: "func NewFoo"
  - test_exists: TestNewFoo
gate:
  assertions:
    - spec_coverage: TestNewFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result.Assertions)
	}
	// 1 file_exists + 1 grep (func_exists) + 1 test_exists + 1 spec_coverage = 4
	if len(result.Assertions) != 4 {
		t.Fatalf("expected 4 assertions (file_exists, func_exists, test_exists, spec_coverage), got %d: %+v", len(result.Assertions), result.Assertions)
	}
}

func TestRun_AllThreeFeatures_SpecCoverageFails(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc NewFoo() {}\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestNewFoo(t *testing.T) {}\n")

	spec := parseSpec(t, `
spec: "implement NewFoo"
complexity: simple
files:
  - foo.go
promises:
  - func_exists: "func NewFoo"
  - test_exists: TestNewFoo
gate:
  assertions:
    - spec_coverage: TestEdgeCaseMissing
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected Passed=false (spec_coverage target not in test files)")
	}
	// Find the failed spec_coverage assertion
	foundCovFail := false
	for _, a := range result.Assertions {
		if !a.Passed && strings.Contains(a.Description, "spec_coverage") {
			foundCovFail = true
		}
	}
	if !foundCovFail {
		t.Errorf("expected a failed spec_coverage assertion in results: %+v", result.Assertions)
	}
}

func TestRun_SpecCoverage_EmptySpec_NoTrigger(t *testing.T) {
	// When SpecCoverage value is a valid identifier and the target exists in test
	// files, spec_coverage passes even when the Spec prose field is empty.
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc Foo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\tif got := Foo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
gate:
  assertions:
    - spec_coverage: TestFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// spec_coverage checks the target text in test files; with empty Spec field
	// the gate still runs the coverage assertion (no AI trigger required).
	if !result.Passed {
		t.Fatalf("expected Passed=true (coverage target found), got false: %+v", result.Assertions)
	}
	if len(result.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(result.Assertions))
	}
}

func TestRun_SpecCoverage_PopulatedSpec_Pass(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n\nfunc Foo() string { return \"foo\" }\n")
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\tif got := Foo(); got == \"\" { t.Fatal(\"empty\") }\n}\n")

	spec := parseSpec(t, `
spec: "Implement Foo with error handling"
complexity: simple
gate:
  assertions:
    - spec_coverage: TestFoo
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true: %+v", result.Assertions)
	}
}

func TestRun_SpecCoverage_PopulatedSpec_Fail(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n")

	spec := parseSpec(t, `
spec: "Implement Foo with edge cases"
complexity: simple
gate:
  assertions:
    - spec_coverage: TestEdgeCaseNotPresent
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected Passed=false (coverage target not in test files)")
	}
}

func TestRun_InvalidWorkdir_Error(t *testing.T) {
	spec := parseSpec(t, `
spec: "test"
complexity: simple
files:
  - test.go
`)
	result, err := Run(context.Background(), spec, "/nonexistent/path/xyz123abc", WithVerifier(false))
	// gate.Run should not error — it runs assertions and reports failures.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// file_exists fails when workdir doesn't exist
	if result.Passed {
		t.Fatal("expected Passed=false for invalid workdir")
	}
}

func TestRun_NilSpec_HandlesGracefully(t *testing.T) {
	result, err := Run(context.Background(), nil, t.TempDir())
	if err == nil {
		t.Fatal("expected error for nil spec")
	}
	if result != nil {
		t.Fatal("expected nil result for nil spec")
	}
}

func TestRun_EmptyWorkdir_String(t *testing.T) {
	spec := parseSpec(t, `
spec: "test"
complexity: simple
files:
  - test.go
`)
	// Empty workdir is treated as relative to current directory.
	// Should not panic; we just verify no panic and get a result.
	result, err := Run(context.Background(), spec, "", WithVerifier(false))
	if err != nil {
		return // error is acceptable for empty workdir
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRun_AllFeaturesEmpty_NoAssertions(t *testing.T) {
	workdir := t.TempDir()
	// Non-empty Spec but no files, promises, or gate assertions.
	spec := parseSpec(t, `
spec: "do something"
complexity: simple
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// No assertions, no checkpoints — passes since spec has content.
	if len(result.Assertions) != 0 {
		t.Fatalf("expected 0 assertions, got %d: %+v", len(result.Assertions), result.Assertions)
	}
	if !result.Passed {
		t.Fatal("expected Passed=true when no assertions to fail")
	}
}

func TestRun_FilesWithEmptyString_Skipped(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "valid.go", "package main\n")
	writeFile(t, workdir, "other.go", "package main\n")

	spec := parseSpec(t, `
spec: "test"
complexity: simple
files:
  - valid.go
  - ""
  - other.go
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should not panic; verify 3 assertions were derived (one per file entry including empty).
	if len(result.Assertions) == 0 {
		t.Fatal("expected at least 1 assertion")
	}
}

func TestRun_FilesDuplicateEntries(t *testing.T) {
	workdir := t.TempDir()
	writeFile(t, workdir, "foo.go", "package main\n")

	spec := parseSpec(t, `
spec: "test"
complexity: simple
files:
  - foo.go
  - foo.go
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true (file exists), got false: %+v", result.Assertions)
	}
	// Duplicate entries result in 2 assertions (deduplication is not performed).
	if len(result.Assertions) != 2 {
		t.Fatalf("expected 2 assertions for duplicate files, got %d", len(result.Assertions))
	}
}

func TestRun_AdapterError_HandledGracefully(t *testing.T) {
	// Simulate spec_coverage assertion where the target is not found.
	// Verifies the gate handles coverage failures gracefully without panicking.
	workdir := t.TempDir()
	writeFile(t, workdir, "foo_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n")

	spec := parseSpec(t, `
spec: "test"
complexity: simple
gate:
  assertions:
    - spec_coverage: TestTargetThatDoesNotExist
`)
	result, err := Run(context.Background(), spec, workdir, WithVerifier(false))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// Coverage target not found → assertion fails, gate fails
	if result.Passed {
		t.Fatal("expected Passed=false when spec_coverage target not found")
	}
	if len(result.Assertions) == 0 {
		t.Fatal("expected at least 1 assertion result")
	}
}
