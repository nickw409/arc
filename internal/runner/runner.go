package runner

import (
	"context"
	"time"
)

// RunOptions configures a test runner invocation.
type RunOptions struct {
	Runner  string
	TestFile string
	Filter  string
	Timeout time.Duration
	ArcHome string
}

// Run executes the appropriate test runner and returns parsed results.
func Run(ctx context.Context, opts RunOptions) (*TestResult, error) {
	panic("not implemented")
}

// SupportedRunners returns the list of known runner names.
func SupportedRunners() []string {
	panic("not implemented")
}

// RunAll executes all test files for a phase and returns aggregated results.
func RunAll(ctx context.Context, runner string, testFiles []string, timeout time.Duration, arcHome string) (*TestResult, error) {
	panic("not implemented")
}
