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
// When specVerifier is non-nil it is treated as authoritative: true enables, false disables.
// When specVerifier is nil the decision falls back to complexity-based auto-detection.
func ShouldRunVerifier(override *bool, configVerifier string, specVerifier *bool, complexity string) bool {
	if override != nil {
		return *override
	}
	switch configVerifier {
	case "always":
		return true
	case "never":
		return false
	default: // "auto" or empty
		if specVerifier != nil {
			return *specVerifier
		}
		return complexity == "complex" || complexity == "medium"
	}
}

// Run executes gate checks for a phase. It accepts an already-parsed spec,
// runs all assertions against workdir, and returns a GateResult.
//
// spec must not be nil.
// workdir is the directory against which file-system assertions are evaluated.
//
// If ctx has no deadline, a default 5-minute timeout is applied.
func Run(ctx context.Context, spec *arc.PhaseSpec, workdir string, opts ...RunOption) (*arc.GateResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("gate.Run: spec must not be nil")
	}

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

	// Build effective assertion list: explicit gate assertions + derived file_exists
	// assertions from spec.Files (for files not already covered by an explicit assertion)
	// + promise-derived assertions.
	effectiveAssertions := append([]arc.GateAssertion(nil), spec.Gate.Assertions...)
	effectiveAssertions = append(effectiveAssertions, deriveFileExistsAssertions(spec.Files, spec.Gate.Assertions)...)
	promiseAssertions, testCoversItems := derivePromiseAssertions(spec.Promises)
	effectiveAssertions = append(effectiveAssertions, promiseAssertions...)

	// Fail fast if the spec has nothing to verify — an empty gate is misconfigured,
	// not passing. Require at least one assertion, checkpoint, or a non-empty spec/verify field.
	hasContent := strings.TrimSpace(spec.Spec) != "" || strings.TrimSpace(spec.Verify) != ""
	if len(effectiveAssertions) == 0 && len(spec.Checkpoints) == 0 && len(spec.Promises) == 0 && !hasContent {
		return &arc.GateResult{
			Passed: false,
			Assertions: []arc.AssertionResult{{
				Passed: false,
				Detail: "gate misconfigured: no assertions, no checkpoints, and no spec defined",
			}},
		}, nil
	}

	result := &arc.GateResult{
		Passed:          true,
		Assertions:      make([]arc.AssertionResult, 0, len(effectiveAssertions)),
		Checkpoints:     make([]arc.CheckpointStatus, 0, len(spec.Checkpoints)),
		TestCoversQueue: testCoversItems,
	}

	// Run assertions.
	for _, a := range effectiveAssertions {
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
		verifyCtx, verifyCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer verifyCancel()
		passed, reasoning, verifyErr := RunVerifier(verifyCtx, spec, workdir)
		if verifyErr != nil {
			// Verifier error is non-fatal — log but don't fail.
			result.ScopedTestSkipped = false
			result.ScopedTestPassed = false
			result.ScopedTestOutput += fmt.Sprintf("Verifier error: %v", verifyErr)
		} else if !passed {
			result.Passed = false
			result.ScopedTestSkipped = false
			result.ScopedTestPassed = false
			result.ScopedTestOutput += fmt.Sprintf("Verifier FAILED:\n%s", reasoning)
		}
	}

	return result, nil
}

// derivePromiseAssertions converts promises into GateAssertions.
// Does not deduplicate against existing assertions.
// Returns assertions and a list of test_covers targets queued for the next phase.
func derivePromiseAssertions(promises []arc.Promise) ([]arc.GateAssertion, []string) {
	var assertions []arc.GateAssertion
	var testCoversItems []string
	for _, p := range promises {
		switch {
		case p.FuncExists != "" && strings.TrimSpace(p.FuncExists) != "":
			assertions = append(assertions, arc.GateAssertion{
				Description: fmt.Sprintf("func_exists: %s", p.FuncExists),
				Grep:        p.FuncExists,
			})
		case p.TestExists != "" && strings.TrimSpace(p.TestExists) != "":
			assertions = append(assertions, arc.GateAssertion{
				Description: fmt.Sprintf("test_exists: %s", p.TestExists),
				TestExists:  p.TestExists,
			})
		case p.FileExists != "" && strings.TrimSpace(p.FileExists) != "":
			assertions = append(assertions, arc.GateAssertion{
				Description: fmt.Sprintf("file_exists: %s (from promise)", p.FileExists),
				FileExists:  p.FileExists,
			})
		case p.TestCovers != "" && p.Test != "":
			assertions = append(assertions, arc.GateAssertion{
				Description: fmt.Sprintf("test_exists: %s (covers %s)", p.Test, p.TestCovers),
				TestExists:  p.Test,
			})
			testCoversItems = append(testCoversItems, p.TestCovers)
		}
	}
	return assertions, testCoversItems
}

// deriveFileExistsAssertions returns file_exists assertions for each path in files
// that is not already covered by an explicit assertion in existing.
func deriveFileExistsAssertions(files []string, existing []arc.GateAssertion) []arc.GateAssertion {
	if len(files) == 0 {
		return nil
	}
	// Build set of paths already covered by explicit file_exists assertions.
	covered := make(map[string]bool, len(existing))
	for _, a := range existing {
		switch {
		case a.FileExists != "":
			covered[a.FileExists] = true
		case a.Type == "file_exists" && a.Target != "":
			covered[a.Target] = true
		}
	}
	var derived []arc.GateAssertion
	for _, f := range files {
		if !covered[f] {
			derived = append(derived, arc.GateAssertion{FileExists: f})
		}
	}
	return derived
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
	case a.FileAbsent != "":
		return checkFileAbsent(desc, a.FileAbsent, workdir)
	case a.GrepNot != "":
		return checkGrepNot(desc, a.GrepNot, workdir)
	case a.NoModified != "":
		return checkNoModified(desc, a.NoModified, workdir)
	case a.FilesOnly != "":
		return checkFilesOnly(desc, a.FilesOnly, workdir)
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

// checkFileAbsent checks that target does NOT exist relative to workdir.
func checkFileAbsent(desc, target, workdir string) arc.AssertionResult {
	full := filepath.Join(workdir, target)
	_, err := os.Stat(full)
	if os.IsNotExist(err) {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("absent (as expected): %s", target),
		}
	}
	if err == nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("file exists but should not: %s", target),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("stat error: %v", err),
	}
}

// checkGrepNot searches all .go files in workdir and fails if pattern is found.
func checkGrepNot(desc, pattern, workdir string) arc.AssertionResult {
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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(content), pattern) {
			found = true
			rel, _ := filepath.Rel(workdir, path)
			matchFile = rel
		}
		return nil
	})
	if err != nil {
		return arc.AssertionResult{Description: desc, Passed: false, Detail: fmt.Sprintf("walk error: %v", err)}
	}
	if found {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("pattern %q found in %s (should not be present)", pattern, matchFile),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      true,
		Detail:      fmt.Sprintf("pattern %q not found in any .go file", pattern),
	}
}

// checkNoModified checks that the given path has no uncommitted changes per git diff HEAD.
func checkNoModified(desc, target, workdir string) arc.AssertionResult {
	cmd := exec.Command("git", "diff", "HEAD", "--", target)
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("git diff failed: %v: %s", err, strings.TrimSpace(buf.String())),
		}
	}
	if buf.Len() > 0 {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("%s was modified", target),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      true,
		Detail:      fmt.Sprintf("%s is unmodified", target),
	}
}

// checkFilesOnly checks that every file in git diff HEAD --name-only matches
// at least one of the comma-separated glob patterns in allowedPatterns.
func checkFilesOnly(desc, allowedPatterns, workdir string) arc.AssertionResult {
	cmd := exec.Command("git", "diff", "HEAD", "--name-only")
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      fmt.Sprintf("git diff failed: %v: %s", err, strings.TrimSpace(buf.String())),
		}
	}

	patterns := strings.Split(allowedPatterns, ",")
	for i, p := range patterns {
		patterns[i] = strings.TrimSpace(p)
	}

	var violations []string
	for _, file := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if !matchesAny(file, patterns) {
			violations = append(violations, file)
		}
	}

	if len(violations) == 0 {
		return arc.AssertionResult{
			Description: desc,
			Passed:      true,
			Detail:      fmt.Sprintf("all changed files match allowed patterns: %s", allowedPatterns),
		}
	}
	return arc.AssertionResult{
		Description: desc,
		Passed:      false,
		Detail:      fmt.Sprintf("files modified outside allowed scope (%s): %s", allowedPatterns, strings.Join(violations, ", ")),
	}
}

// matchesAny reports whether file matches at least one of the given glob patterns.
// Supports "prefix/**" (anything under prefix/) and standard filepath.Match patterns.
func matchesAny(file string, patterns []string) bool {
	for _, p := range patterns {
		if matchGlob(file, p) {
			return true
		}
	}
	return false
}

// matchGlob matches a file path against a glob pattern.
// Supports "prefix/**" meaning anything rooted under prefix/.
func matchGlob(file, pattern string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	matched, _ := filepath.Match(pattern, file)
	return matched
}

// HasAssertions reads a spec YAML and returns true if it defines any gate
// assertions or checkpoints with test commands.
func HasAssertions(specPath string) (bool, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return false, fmt.Errorf("reading spec file %q: %w", specPath, err)
	}

	var spec arc.PhaseSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return false, fmt.Errorf("parsing spec file %q: %w", specPath, err)
	}

	if len(spec.Gate.Assertions) > 0 {
		return true, nil
	}
	for _, cp := range spec.Checkpoints {
		if cp.Test != "" {
			return true, nil
		}
	}
	return false, nil
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
