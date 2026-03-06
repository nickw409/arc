package gate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeSpec writes a spec.yaml into dir and returns its path.
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
	phaseDir := t.TempDir()

	// Create the target file.
	writeFile(t, workdir, "internal/api/auth.go", "package api\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "auth.go exists"
      file_exists: internal/api/auth.go
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "missing.go exists"
      file_exists: internal/missing.go
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	writeFile(t, workdir, "internal/api/auth.go", "package api\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "NewMiddleware exists"
      grep: "func NewMiddleware"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "true exits 0"
      build_passes: "true"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "false exits 1"
      build_passes: "false"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions:
    - description: "build via type field"
      type: build_passes
      target: "true"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_BuildPasses_OutputCaptured(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

	// A command that produces output and fails.
	spec := `
name: test-phase
gate:
  assertions:
    - description: "failing build with output"
      build_passes: "echo 'syntax error'; exit 1"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got: %v", result.Assertions[0].Detail)
	}
}

func TestRun_NoUntracked_Fail_TmpFile(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected Passed=false for debug_ prefixed file")
	}
}

func TestRun_NoUntracked_Pass_NormalUntrackedFile(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true for non-suspicious untracked file, got: %s", result.Assertions[0].Detail)
	}
}

func TestRun_NoUntracked_TypeField(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	writeFile(t, workdir, "myfile.go", "package main\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "myfile.go via type field"
      type: file_exists
      target: myfile.go
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_TypeTarget_Grep(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

	writeFile(t, workdir, "main.go", "package main\n\nfunc main() {}\n")

	spec := `
name: test-phase
gate:
  assertions:
    - description: "func main via type field"
      type: grep
      target: "func main"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestRun_TypeTarget_TestExists(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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

func TestRun_MissingSpec(t *testing.T) {
	workdir := t.TempDir()
	_, err := Run(context.Background(), filepath.Join(workdir, "nonexistent.yaml"), workdir)
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — empty assertions (should pass)
// ---------------------------------------------------------------------------

func TestRun_EmptyAssertions_Pass(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true for empty assertions")
	}
	if len(result.Assertions) != 0 {
		t.Errorf("expected 0 assertions, got %d", len(result.Assertions))
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
verify: "Ensure all API endpoints return 200"
gate:
  assertions: []
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.ScopedTestSkipped {
		t.Errorf("expected ScopedTestSkipped=true when no test command set")
	}
}

// ---------------------------------------------------------------------------
// Run — checkpoint test commands
// ---------------------------------------------------------------------------

func TestRun_Checkpoint_Pass(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "Always passing"
    test: "true"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "Always failing"
    test: "false"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

	spec := `
name: test-phase
gate:
  assertions: []
checkpoints:
  - name: cp1
    description: "No test command"
`
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	r1, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	r2, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
	phaseDir := t.TempDir()

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
	specPath := writeSpec(t, phaseDir, spec)

	result, err := Run(context.Background(), specPath, workdir)
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
		specVerifier   bool
		complexity     string
		want           bool
	}{
		{"override true wins", &tr, "never", false, "simple", true},
		{"override false wins", &fa, "always", true, "complex", false},
		{"config always", nil, "always", false, "simple", true},
		{"config never", nil, "never", true, "complex", false},
		{"auto + complex", nil, "auto", false, "complex", true},
		{"auto + medium", nil, "auto", false, "medium", true},
		{"auto + simple", nil, "auto", false, "simple", false},
		{"auto + spec true", nil, "auto", true, "simple", true},
		{"auto + empty complexity", nil, "auto", false, "", false},
		{"empty config + complex", nil, "", false, "complex", true},
		{"empty config + simple", nil, "", false, "simple", false},
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
