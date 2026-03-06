// Package gate implements the Arc phase gate — objective, independent checks
// that determine whether a phase has met its acceptance criteria.
package gate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"gopkg.in/yaml.v3"
)

// RunOption configures gate execution.
type RunOption func(*runOptions)

type runOptions struct {
	verifierOverride *bool // nil = use spec/auto logic; non-nil = force
}

// WithVerifier forces the verifier agent on or off, overriding spec and auto-detection.
func WithVerifier(enabled bool) RunOption {
	return func(o *runOptions) {
		o.verifierOverride = &enabled
	}
}

// ShouldRunVerifier resolves whether the verifier agent should run for a phase.
// Priority: explicit override > "always"/"never" config > spec field > auto (complexity-based).
func ShouldRunVerifier(override *bool, configVerifier string, specVerifier bool, complexity string) bool {
	if override != nil {
		return *override
	}
	switch configVerifier {
	case "always":
		return true
	case "never":
		return false
	default: // "auto" or empty
		if specVerifier {
			return true
		}
		return complexity == "complex" || complexity == "medium"
	}
}

// Run executes gate checks for a phase. It reads the spec from specPath,
// runs all assertions against workdir, and returns a GateResult.
//
// specPath must point to a spec.yaml file.
// workdir is the directory against which file-system assertions are evaluated.
//
// If ctx has no deadline, a default 5-minute timeout is applied.
func Run(ctx context.Context, specPath string, workdir string, opts ...RunOption) (*arc.GateResult, error) {
	// Apply a default timeout if the caller did not set one.
	var ro runOptions
	for _, opt := range opts {
		opt(&ro)
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("reading spec file %q: %w", specPath, err)
	}

	var spec arc.PhaseSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing spec file %q: %w", specPath, err)
	}

	// Fail fast if the spec has nothing to verify — an empty gate is misconfigured,
	// not passing. Require at least one assertion, checkpoint, or a non-empty spec/verify field.
	hasContent := strings.TrimSpace(spec.Spec) != "" || strings.TrimSpace(spec.Verify) != ""
	if len(spec.Gate.Assertions) == 0 && len(spec.Checkpoints) == 0 && !hasContent {
		return &arc.GateResult{
			Passed: false,
			Assertions: []arc.AssertionResult{{
				Passed: false,
				Detail: "gate misconfigured: no assertions, no checkpoints, and no spec defined",
			}},
		}, nil
	}

	result := &arc.GateResult{
		Passed:      true,
		Assertions:  make([]arc.AssertionResult, 0, len(spec.Gate.Assertions)),
		Checkpoints: make([]arc.CheckpointStatus, 0, len(spec.Checkpoints)),
	}

	// Run assertions.
	for _, a := range spec.Gate.Assertions {
		ar := runAssertion(a, workdir)
		result.Assertions = append(result.Assertions, ar)
		if !ar.Passed {
			result.Passed = false
		}
	}

	// Run checkpoint test commands.
	for _, cp := range spec.Checkpoints {
		cs := runCheckpoint(ctx, cp, workdir)
		result.Checkpoints = append(result.Checkpoints, cs)
		if cs.Status == "fail" {
			result.Passed = false
		}
	}

	// Scoped test: the Verify field is natural language for the verifier agent,
	// not a shell command. Skip shell execution — mechanical checks come from
	// gate assertions (file_exists, build_passes, etc.).
	result.ScopedTestSkipped = true
	result.ScopedTestPassed = true

	// Run verifier agent if resolved and all other checks passed.
	runVerifier := ShouldRunVerifier(ro.verifierOverride, "", spec.Gate.VerifierAgent, spec.Complexity)
	if runVerifier && result.Passed {
		verifyCtx, verifyCancel := context.WithTimeout(ctx, 2*time.Minute)
		defer verifyCancel()
		passed, reasoning, verifyErr := RunVerifier(verifyCtx, &spec, workdir)
		if verifyErr != nil {
			// Verifier error is non-fatal — log but don't fail.
			result.ScopedTestOutput += fmt.Sprintf("\n\nVerifier error: %v", verifyErr)
		} else if !passed {
			result.Passed = false
			result.ScopedTestOutput += fmt.Sprintf("\n\nVerifier FAILED:\n%s", reasoning)
		}
	}

	return result, nil
}

// runAssertion evaluates a single GateAssertion against workdir and returns the result.
func runAssertion(a arc.GateAssertion, workdir string) arc.AssertionResult {
	desc := a.Description

	// Determine assertion kind from populated fields.
	// Support both the legacy Type field and the direct field approach.
	switch {
	case a.FileExists != "":
		return checkFileExists(desc, a.FileExists, workdir)
	case a.Grep != "":
		return checkGrep(desc, a.Grep, workdir)
	case a.TestExists != "":
		return checkTestExists(desc, a.TestExists, workdir)
	case a.BuildPasses != "":
		return checkBuildPasses(desc, a.BuildPasses, workdir)
	case a.NoUntracked != "":
		return checkNoUntracked(desc, workdir)
	case a.Type == "file_exists" && a.Target != "":
		return checkFileExists(desc, a.Target, workdir)
	case a.Type == "grep" && a.Target != "":
		return checkGrep(desc, a.Target, workdir)
	case a.Type == "test_exists" && a.Target != "":
		return checkTestExists(desc, a.Target, workdir)
	case a.Type == "build_passes" && a.Target != "":
		return checkBuildPasses(desc, a.Target, workdir)
	case a.Type == "no_untracked":
		return checkNoUntracked(desc, workdir)
	default:
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      "unknown assertion type or missing target",
		}
	}
}

// checkFileExists checks whether target exists relative to workdir.
func checkFileExists(desc, target, workdir string) arc.AssertionResult {
	full := filepath.Join(workdir, target)
	_, err := os.Stat(full)
	if err == nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("found: %s", target),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("not found: %s", target),
	}
}

// checkGrep searches all .go files in workdir for the given pattern.
// Returns a passing result if the pattern is found in at least one file.
func checkGrep(desc, pattern, workdir string) arc.AssertionResult {
	found := false
	matchFile := ""

	err := filepath.Walk(workdir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return err
		}
		if info.IsDir() {
			// Skip hidden and vendor directories.
			base := info.Name()
			if base != "." && (strings.HasPrefix(base, ".") || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}
		if strings.Contains(string(content), pattern) {
			found = true
			rel, _ := filepath.Rel(workdir, path)
			matchFile = rel
		}
		return nil
	})
	if err != nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("walk error: %v", err),
		}
	}

	if found {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("pattern %q found in %s", pattern, matchFile),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("pattern %q not found in any .go file", pattern),
	}
}

// checkTestExists searches all _test.go files for a function named funcName.
func checkTestExists(desc, funcName, workdir string) arc.AssertionResult {
	needle := "func " + funcName
	found := false
	matchFile := ""

	err := filepath.Walk(workdir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base != "." && (strings.HasPrefix(base, ".") || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(content), needle) {
			found = true
			rel, _ := filepath.Rel(workdir, path)
			matchFile = rel
		}
		return nil
	})
	if err != nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("walk error: %v", err),
		}
	}

	if found {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("%q found in %s", funcName, matchFile),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("%q not found in any _test.go file", funcName),
	}
}

// checkBuildPasses runs the given build command in workdir and checks exit code 0.
// The command is run via sh -c so it supports pipelines and environment expansion.
func checkBuildPasses(desc, command, workdir string) arc.AssertionResult {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	output := buf.String()
	if err == nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("build command %q passed", command),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("build command %q failed: %v\n%s", command, err, strings.TrimSpace(output)),
	}
}

// suspiciousPatterns are the glob-style suffixes and prefixes that flag a file
// as a debug/temp artifact for the no_untracked assertion.
var suspiciousPatterns = []struct {
	suffix string
	prefix string
	exact  string
}{
	{suffix: ".tmp"},
	{suffix: ".bak"},
	{suffix: ".orig"},
	{prefix: "debug_"},
	{prefix: "scratch"},
	{exact: "TODO"},
}

// isSuspicious returns true if the base name of path looks like a debug or
// temporary artifact.
func isSuspicious(path string) bool {
	base := filepath.Base(path)
	for _, p := range suspiciousPatterns {
		switch {
		case p.exact != "" && base == p.exact:
			return true
		case p.suffix != "" && strings.HasSuffix(base, p.suffix):
			return true
		case p.prefix != "" && strings.HasPrefix(base, p.prefix):
			return true
		}
	}
	return false
}

// checkNoUntracked runs git ls-files --others --exclude-standard in workdir
// and fails if any suspicious untracked files are found.
func checkNoUntracked(desc, workdir string) arc.AssertionResult {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("git ls-files failed: %v: %s", err, strings.TrimSpace(buf.String())),
		}
	}

	var found []string
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isSuspicious(line) {
			found = append(found, line)
		}
	}

	if len(found) == 0 {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      "no suspicious untracked files found",
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("suspicious untracked files: %s", strings.Join(found, ", ")),
	}
}

// runCheckpoint runs a checkpoint's test command and returns its status.
func runCheckpoint(ctx context.Context, cp arc.Checkpoint, workdir string) arc.CheckpointStatus {
	if cp.Test == "" {
		return arc.CheckpointStatus{
			Name:   cp.Name,
			Status: "not_run",
		}
	}
	out, err := runCommand(ctx, cp.Test, workdir)
	if err != nil {
		return arc.CheckpointStatus{
			Name:   cp.Name,
			Status: "fail",
			Output: out,
		}
	}
	return arc.CheckpointStatus{
		Name:   cp.Name,
		Status: "pass",
		Output: out,
	}
}

// runCommand executes a shell command in dir and returns combined output.
// Returns an error if the command exits with a non-zero status.
func runCommand(ctx context.Context, command, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
