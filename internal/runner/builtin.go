package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RunBuiltinOptions configures a built-in test run.
type RunBuiltinOptions struct {
	TestFile    string        // path to test file (required)
	Filter      string        // optional: run only tests matching this pattern
	Timeout     time.Duration // per-run timeout (default 5 min)
	Dir         string        // working directory
	Language    string        // from .arc.yaml
	Runner      string        // from .arc.yaml (e.g., "go-test")
	TestCommand string        // from .arc.yaml (e.g., "go test") — overrides Runner if set
}

// RunBuiltin runs scoped tests for a specific file using config from .arc.yaml.
func RunBuiltin(ctx context.Context, opts RunBuiltinOptions) (*TestResult, error) {
	if opts.TestFile == "" {
		return nil, fmt.Errorf("test file is required")
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	args, runner, err := buildCommand(opts, timeout)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, args[0], args[1:]...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("executing test command: %w", runErr)
		}
	}

	result := parseTestOutput(runner, stdout.String(), stderr.String(), exitCode)
	result.Duration = duration
	return result, nil
}

// buildCommand constructs the command args for the given options.
func buildCommand(opts RunBuiltinOptions, timeout time.Duration) (args []string, runner string, err error) {
	if opts.TestCommand != "" {
		parts := strings.Fields(opts.TestCommand)
		if len(parts) == 0 {
			return nil, "", fmt.Errorf("empty test_command")
		}
		parts = append(parts, opts.TestFile)
		if opts.Filter != "" {
			parts = append(parts, "-run", opts.Filter)
		}
		return parts, "custom", nil
	}

	runner = opts.Runner
	switch runner {
	case "go-test":
		return buildGoTestCommand(opts, timeout), runner, nil
	default:
		return nil, "", fmt.Errorf("unsupported runner: %s (use test_command override or wait for multi-language support)", runner)
	}
}

func buildGoTestCommand(opts RunBuiltinOptions, timeout time.Duration) []string {
	dir := filepath.ToSlash(filepath.Dir(opts.TestFile))
	var pkg string
	switch {
	case filepath.IsAbs(opts.TestFile):
		pkg = dir + "/"
	case dir == ".":
		pkg = "./"
	default:
		pkg = "./" + dir + "/"
	}
	args := []string{"go", "test", pkg, "-v", "-count=1"}
	if opts.Filter != "" {
		args = append(args, "-run", opts.Filter)
	}
	args = append(args, "-timeout", fmt.Sprintf("%s", timeout))
	return args
}

// parseTestOutput parses runner output into a TestResult based on runner type.
func parseTestOutput(runner string, stdout string, stderr string, exitCode int) *TestResult {
	combined := stdout + "\n" + stderr
	result := &TestResult{
		RawOutput:   combined,
		FailedNames: []string{},
	}

	switch runner {
	case "go-test":
		parseGoTestOutput(result, stdout, stderr, exitCode)
	default:
		// For custom/unknown runners, fall back to exit-code-based pass/fail.
		if exitCode == 0 {
			result.Total = 1
			result.Passed = 1
		} else {
			result.Total = 1
			result.Failed = 1
		}
	}

	return result
}

var (
	goPassRe     = regexp.MustCompile(`--- PASS: (\S+)`)
	goFailRe     = regexp.MustCompile(`--- FAIL: (\S+)`)
	goTimingRe   = regexp.MustCompile(`ok\s+\S+\s+([\d.]+)s`)
)

func parseGoTestOutput(result *TestResult, stdout string, stderr string, exitCode int) {
	lines := strings.Split(stdout+"\n"+stderr, "\n")

	var passNames, failNames []string
	for _, line := range lines {
		if m := goPassRe.FindStringSubmatch(line); m != nil {
			passNames = append(passNames, m[1])
		}
		if m := goFailRe.FindStringSubmatch(line); m != nil {
			failNames = append(failNames, m[1])
		}
	}

	// Deduplicate: if TestFoo/sub failed, TestFoo also shows as FAIL.
	// Keep only the most specific names (subtests), not their parents.
	failNames = deduplicateSubtests(failNames)
	passNames = deduplicateSubtests(passNames)

	result.Passed = len(passNames)
	result.Failed = len(failNames)
	result.FailedNames = failNames
	result.Total = result.Passed + result.Failed

	// Parse timing from "ok" line
	if m := goTimingRe.FindStringSubmatch(stdout); m != nil {
		if secs, err := strconv.ParseFloat(m[1], 64); err == nil {
			result.Duration = time.Duration(secs * float64(time.Second))
		}
	}

	// No test lines but non-zero exit → compilation error
	if result.Total == 0 && exitCode != 0 {
		result.Failed = 1
		result.Total = 1
	}
}

// deduplicateSubtests removes parent test names when a subtest is present.
// e.g., [TestFoo/sub2, TestFoo] → [TestFoo/sub2] because TestFoo only
// appears as FAIL because its subtest failed.
func deduplicateSubtests(names []string) []string {
	if len(names) <= 1 {
		return names
	}
	var result []string
	for _, n := range names {
		// Keep this name only if no other name has it as a prefix (parent)
		isParent := false
		for _, other := range names {
			if other != n && strings.HasPrefix(other, n+"/") {
				isParent = true
				break
			}
		}
		if !isParent {
			result = append(result, n)
		}
	}
	return result
}
