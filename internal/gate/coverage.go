package gate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
)

// collectTestFiles returns the paths of all _test.go files found under workdir.
// Hidden directories and vendor/ are skipped.
func collectTestFiles(workdir string) ([]string, error) {
	var testFiles []string
	err := filepath.Walk(workdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base != "." && (strings.HasPrefix(base, ".") || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			testFiles = append(testFiles, path)
		}
		return nil
	})
	return testFiles, err
}

// RunSpecCoverage evaluates spec_coverage assertions using an AI agent that
// reads the spec and test files to determine whether each target is meaningfully
// tested. Assertions without a SpecCoverage value are ignored.
// Returns nil when no spec_coverage assertions are present.
func RunSpecCoverage(ctx context.Context, spec *arc.PhaseSpec, assertions []arc.GateAssertion, workdir string, commandName string) []arc.AssertionResult {
	var coverageAssertions []arc.GateAssertion
	for _, a := range assertions {
		if a.SpecCoverage != "" {
			coverageAssertions = append(coverageAssertions, a)
		}
	}
	if len(coverageAssertions) == 0 {
		return nil
	}

	testFiles, err := collectTestFiles(workdir)
	if err != nil {
		return []arc.AssertionResult{{
			Description: "spec_coverage",
			Passed:      false,
			Detail:      fmt.Sprintf("collecting test files: %v", err),
		}}
	}

	// Build numbered target list for the prompt.
	var targetLines []string
	for i, a := range coverageAssertions {
		targetLines = append(targetLines, fmt.Sprintf("%d. %s", i+1, a.SpecCoverage))
	}

	// Build test file list for the prompt.
	var fileLines []string
	for _, f := range testFiles {
		rel, _ := filepath.Rel(workdir, f)
		fileLines = append(fileLines, rel)
	}
	fileList := strings.Join(fileLines, "\n")
	if fileList == "" {
		fileList = "(none found)"
	}

	prompt := fmt.Sprintf(`You are a test coverage reviewer. Determine whether each target is meaningfully exercised by the existing test suite.

## Specification
%s

## Targets to check
%s

## Test files available
%s

## Instructions
Use Read and Grep tools to inspect the test files above. For each numbered target:
- Determine if there is at least one test that directly exercises that function, type, or behavior
- A test "covers" a target if it calls or constructs the target — not just if the name appears in a comment or import
- Return a verdict for EVERY target

Respond with exactly one line per target in this format (no other text):
PASS <number>: <target> — <one-line reason>
FAIL <number>: <target> — <one-line reason>
`, spec.Spec, strings.Join(targetLines, "\n"), fileList)

	var agentAdapter arc.AgentAdapter
	if commandName != "" {
		agentAdapter = &adapter.ClaudeAdapter{CommandName: commandName}
	} else {
		agentAdapter = adapter.Get("claude")
	}

	sessionCfg := arc.SessionConfig{
		MaxTurns: 20,
		Timeout:  5 * time.Minute,
		Tools:    []string{"Read", "Grep", "Glob"},
	}

	agentResult, spawnErr := agentAdapter.Spawn(ctx, prompt, workdir, sessionCfg)
	if spawnErr != nil {
		var results []arc.AssertionResult
		for _, a := range coverageAssertions {
			desc := a.Description
			if desc == "" {
				desc = fmt.Sprintf("spec_coverage: %s", a.SpecCoverage)
			}
			results = append(results, arc.AssertionResult{
				Description: desc,
				Passed:      false,
				Detail:      fmt.Sprintf("coverage agent failed: %v", spawnErr),
			})
		}
		return results
	}

	output := ""
	if agentResult != nil {
		output = agentResult.Output
	}

	return parseSpecCoverageOutput(output, coverageAssertions)
}

// parseSpecCoverageOutput maps agent verdict lines back to assertion results.
// Expected line format: "PASS N: <target> — <reason>" or "FAIL N: <target> — <reason>"
func parseSpecCoverageOutput(output string, assertions []arc.GateAssertion) []arc.AssertionResult {
	results := make([]arc.AssertionResult, len(assertions))
	for i, a := range assertions {
		desc := a.Description
		if desc == "" {
			desc = fmt.Sprintf("spec_coverage: %s", a.SpecCoverage)
		}
		results[i] = arc.AssertionResult{
			Description: desc,
			Passed:      false,
			Detail:      "no verdict returned by coverage agent",
		}
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		var pass bool
		var rest string
		if strings.HasPrefix(line, "PASS ") {
			pass = true
			rest = strings.TrimPrefix(line, "PASS ")
		} else if strings.HasPrefix(line, "FAIL ") {
			pass = false
			rest = strings.TrimPrefix(line, "FAIL ")
		} else {
			continue
		}

		// Parse "N: target — reason"
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			continue
		}
		numStr := strings.TrimSpace(rest[:colonIdx])
		afterColon := strings.TrimSpace(rest[colonIdx+1:])

		var idx int
		if _, err := fmt.Sscanf(numStr, "%d", &idx); err != nil || idx < 1 || idx > len(assertions) {
			continue
		}
		idx-- // convert to 0-based

		detail := afterColon
		if dashIdx := strings.Index(afterColon, "—"); dashIdx >= 0 {
			detail = strings.TrimSpace(afterColon[dashIdx+len("—"):])
		}

		results[idx].Passed = pass
		results[idx].Detail = detail
	}

	return results
}

// readFileContent reads a file and returns its content as a string.
// Returns empty string on error.
func readFileContent(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
