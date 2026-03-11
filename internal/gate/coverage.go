package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// RunSpecCoverage evaluates spec_coverage assertions by checking that each
// target string appears in at least one _test.go file under workdir.
// Assertions without a SpecCoverage value are ignored.
// Returns nil when no spec_coverage assertions are present.
func RunSpecCoverage(assertions []arc.GateAssertion, workdir string) []arc.AssertionResult {
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

	// Read all test file contents once to avoid repeated I/O.
	testContents := make(map[string]string, len(testFiles))
	for _, f := range testFiles {
		content, readErr := os.ReadFile(f)
		if readErr == nil {
			rel, _ := filepath.Rel(workdir, f)
			testContents[rel] = string(content)
		}
	}

	var results []arc.AssertionResult
	for _, a := range coverageAssertions {
		target := a.SpecCoverage
		desc := a.Description
		if desc == "" {
			desc = fmt.Sprintf("spec_coverage: %s", target)
		}

		found := false
		foundIn := ""
		for relPath, content := range testContents {
			if strings.Contains(content, target) {
				found = true
				foundIn = relPath
				break
			}
		}

		if found {
			results = append(results, arc.AssertionResult{
				Description: desc,
				Passed:      true,
				Detail:      fmt.Sprintf("%q covered in %s", target, foundIn),
			})
		} else {
			results = append(results, arc.AssertionResult{
				Description: desc,
				Passed:      false,
				Detail:      fmt.Sprintf("%q not found in any _test.go file", target),
			})
		}
	}
	return results
}
