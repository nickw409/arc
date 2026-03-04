// Package gate implements the Arc phase gate — objective, independent checks
// that determine whether a phase has met its acceptance criteria.
package gate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"gopkg.in/yaml.v3"
)

// Run executes gate checks for a phase. It reads the spec from specPath,
// runs all assertions against workdir, and returns a GateResult.
//
// specPath must point to a spec.yaml file.
// workdir is the directory against which file-system assertions are evaluated.
func Run(specPath string, workdir string) (*arc.GateResult, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("reading spec file %q: %w", specPath, err)
	}

	var spec arc.PhaseSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing spec file %q: %w", specPath, err)
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
		cs := runCheckpoint(cp, workdir)
		result.Checkpoints = append(result.Checkpoints, cs)
		if cs.Status == "fail" {
			result.Passed = false
		}
	}

	// Run scoped test command.
	if spec.Test == "" {
		result.ScopedTestSkipped = true
		result.ScopedTestPassed = true // nothing to fail
	} else {
		out, testErr := runCommand(spec.Test, workdir)
		result.ScopedTestOutput = out
		if testErr != nil {
			result.ScopedTestPassed = false
			result.Passed = false
		} else {
			result.ScopedTestPassed = true
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
	case a.Type == "file_exists" && a.Target != "":
		return checkFileExists(desc, a.Target, workdir)
	case a.Type == "grep" && a.Target != "":
		return checkGrep(desc, a.Target, workdir)
	case a.Type == "test_exists" && a.Target != "":
		return checkTestExists(desc, a.Target, workdir)
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

// runCheckpoint runs a checkpoint's test command and returns its status.
func runCheckpoint(cp arc.Checkpoint, workdir string) arc.CheckpointStatus {
	if cp.Test == "" {
		return arc.CheckpointStatus{
			Name:   cp.Name,
			Status: "not_run",
		}
	}
	out, err := runCommand(cp.Test, workdir)
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
func runCommand(command, dir string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
