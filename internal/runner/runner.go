package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RunOptions configures a test runner invocation.
type RunOptions struct {
	Runner   string
	TestFile string
	Filter   string
	Timeout  time.Duration
	ArcHome  string
}

// Run executes the appropriate test runner and returns parsed results.
func Run(ctx context.Context, opts RunOptions) (*TestResult, error) {
	scriptPath := filepath.Join(opts.ArcHome, "runners", opts.Runner, "run.sh")

	info, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("runner %q not found: %s does not exist", opts.Runner, scriptPath)
		}
		return nil, fmt.Errorf("runner %q: %w", opts.Runner, err)
	}

	if info.Mode()&0111 == 0 {
		return nil, fmt.Errorf("runner %q: permission denied, script is not executable: %s", opts.Runner, scriptPath)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{opts.TestFile}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", int(opts.Timeout.Seconds())))
	}

	cmd := exec.CommandContext(timeoutCtx, scriptPath, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err = cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("runner %q execution failed: %w", opts.Runner, err)
	}
	out := stdout.Bytes()

	var result TestResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("runner %q: failed to parse JSON output: %w", opts.Runner, err)
	}

	return &result, nil
}

// SupportedRunners returns the list of known runner names.
func SupportedRunners() []string {
	return []string{"go-test", "cargo-nextest", "vitest", "pytest", "cargo-test"}
}

// RunAll executes all test files for a phase concurrently and returns aggregated results.
func RunAll(ctx context.Context, runnerName string, testFiles []string, timeout time.Duration, arcHome string) (*TestResult, error) {
	agg := &TestResult{
		FailedNames: []string{},
	}

	if len(testFiles) == 0 {
		return agg, nil
	}

	type fileResult struct {
		result *TestResult
		err    error
	}

	results := make([]fileResult, len(testFiles))
	var wg sync.WaitGroup

	for i, tf := range testFiles {
		wg.Add(1)
		go func(idx int, testFile string) {
			defer wg.Done()
			r, err := Run(ctx, RunOptions{
				Runner:   runnerName,
				TestFile: testFile,
				Timeout:  timeout,
				ArcHome:  arcHome,
			})
			results[idx] = fileResult{result: r, err: err}
		}(i, tf)
	}

	wg.Wait()

	var rawOutputs []string
	for _, fr := range results {
		if fr.err != nil {
			return agg, fr.err
		}
		agg.Total += fr.result.Total
		agg.Passed += fr.result.Passed
		agg.Failed += fr.result.Failed
		agg.FailedNames = append(agg.FailedNames, fr.result.FailedNames...)
		rawOutputs = append(rawOutputs, fr.result.RawOutput)
	}

	agg.RawOutput = strings.Join(rawOutputs, "\n")
	return agg, nil
}
