package gate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ---------------------------------------------------------------------------
// Mock adapter for verifier tests
// ---------------------------------------------------------------------------

// mockAdapter implements arc.AgentAdapter for testing.
type mockAdapter struct {
	output string
	err    error
}

func (m *mockAdapter) Name() string { return "mock" }

func (m *mockAdapter) Spawn(_ context.Context, _ string, _ string, _ arc.SessionConfig) (*arc.AgentResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &arc.AgentResult{Output: m.output}, nil
}

func (m *mockAdapter) Preflight(_ context.Context, _ string) error { return nil }

// ---------------------------------------------------------------------------
// parseVerifierOutput tests — validate PASS/FAIL parsing logic directly
// ---------------------------------------------------------------------------

func TestParseVerifierOutput_Pass(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"exact PASS", "PASS"},
		{"PASS with reasoning", "PASS\nThe implementation is complete and correct."},
		{"pass lowercase", "pass\nLooks good."},
		{"PASS with trailing space", "PASS  \nreasoning"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upper := parseFirstLineUpper(tc.output)
			if !hasPassPrefix(upper) {
				t.Errorf("expected PASS prefix for output %q, got first-line %q", tc.output, upper)
			}
		})
	}
}

func TestParseVerifierOutput_Fail(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"exact FAIL", "FAIL"},
		{"FAIL with reasoning", "FAIL\nMissing the required function."},
		{"fail lowercase", "fail\nBad implementation."},
		{"empty string", ""},
		{"unrecognized prefix", "UNKNOWN\nstuff"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upper := parseFirstLineUpper(tc.output)
			if hasPassPrefix(upper) {
				t.Errorf("expected no PASS prefix for output %q, got first-line %q", tc.output, upper)
			}
		})
	}
}

// parseFirstLineUpper and hasPassPrefix mirror the logic inside RunVerifier so
// we can test the parsing logic without spawning a real agent.
func parseFirstLineUpper(output string) string {
	import_strings_trimspace := func(s string) string {
		// Simple inline whitespace trim without importing strings in this helper.
		start := 0
		for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
			start++
		}
		end := len(s)
		for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
			end--
		}
		return s[start:end]
	}
	trimmed := import_strings_trimspace(output)
	upper := ""
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' {
			upper += string(r - 32)
		} else {
			upper += string(r)
		}
	}
	return upper
}

func hasPassPrefix(upper string) bool {
	return len(upper) >= 4 && upper[:4] == "PASS"
}

// ---------------------------------------------------------------------------
// RunVerifier integration-style tests using a git repo
// ---------------------------------------------------------------------------

// initGitRepo initialises a bare git repo in dir and makes an initial commit
// so that "git diff HEAD" is valid.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	// Create an initial commit so HEAD exists.
	placeholder := filepath.Join(dir, ".gitkeep")
	if err := os.WriteFile(placeholder, []byte(""), 0o644); err != nil {
		t.Fatalf("writing placeholder: %v", err)
	}
	run("add", ".gitkeep")
	run("commit", "-m", "init")
}

func TestGetDiff_NoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	diff, err := getDiff(dir)
	if err != nil {
		t.Fatalf("getDiff: %v", err)
	}
	// After a clean commit there should be no diff.
	if diff != "" {
		t.Errorf("expected empty diff, got: %q", diff)
	}
}

func TestGetDiff_WithChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Make a change to a tracked file.
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("modifying file: %v", err)
	}

	diff, err := getDiff(dir)
	if err != nil {
		t.Fatalf("getDiff: %v", err)
	}
	if diff == "" {
		t.Error("expected non-empty diff after modifying tracked file")
	}
}

func TestGetDiff_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	// No git init — getDiff should return an error.
	_, err := getDiff(dir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

// ---------------------------------------------------------------------------
// RunVerifier with a controlled workdir — verifies the no-diff early exit
// ---------------------------------------------------------------------------

func TestRunVerifier_NoDiff_ReturnsPass(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	spec := &arc.PhaseSpec{
		Name: "test-phase",
		Spec: "Implement a simple function",
	}

	passed, reasoning, err := RunVerifier(context.Background(), spec, dir)
	if err != nil {
		t.Fatalf("RunVerifier: %v", err)
	}
	if !passed {
		t.Errorf("expected passed=true for empty diff, got false (reasoning: %s)", reasoning)
	}
	if reasoning != "no changes to verify" {
		t.Errorf("expected 'no changes to verify', got %q", reasoning)
	}
}

// TestRunVerifier_SkipsWhenNoDiff verifies that RunVerifier returns early
// before spawning an agent when there is no diff.
func TestRunVerifier_SkipsAgentWhenNoDiff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	spec := &arc.PhaseSpec{
		Name: "test-phase",
		Spec: "some spec",
	}

	// Even with a cancelled context, should still return pass (early exit before agent).
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	passed, reasoning, err := RunVerifier(cancelledCtx, spec, dir)
	if err != nil {
		t.Fatalf("RunVerifier: %v", err)
	}
	if !passed {
		t.Errorf("expected passed=true for empty diff (early exit), got false: %s", reasoning)
	}
}

// ---------------------------------------------------------------------------
// gate.Run integration: VerifierAgent=false skips verifier
// ---------------------------------------------------------------------------

func TestRun_VerifierAgent_False_Skipped(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()

	spec := `
name: test-phase
spec: "Implement something"
gate:
  assertions: []
  verifier_agent: false
`
	specPath := writeSpec(t, phaseDir, spec)

	// Should pass with no assertions and verifier disabled.
	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
}

// TestRun_VerifierAgent_True_NoDiff verifies that when verifier_agent=true but
// there is no diff (not a git repo or no changes), Run still completes without error.
func TestRun_VerifierAgent_True_NoDiff(t *testing.T) {
	workdir := t.TempDir()
	phaseDir := t.TempDir()
	initGitRepo(t, workdir)

	spec := `
name: test-phase
spec: "Implement something"
gate:
  assertions: []
  verifier_agent: true
`
	specPath := writeSpec(t, phaseDir, spec)

	// With no diff, RunVerifier returns (true, "no changes to verify", nil)
	// so gate.Run should still pass.
	result, err := Run(context.Background(), specPath, workdir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true (no diff → verifier skipped), got false; output: %s", result.ScopedTestOutput)
	}
}
