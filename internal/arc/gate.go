package arc

import "time"

// GateAssertion defines a single verifiable condition for a phase gate.
type GateAssertion struct {
	// Type is the assertion kind: "file_exists", "grep", or "test_exists".
	Type string `yaml:"type"`
	// Description is a human-readable label for this assertion.
	Description string `yaml:"description"`
	// Target is the file path (for file_exists), pattern (for grep), or
	// function name (for test_exists).
	Target string `yaml:"target"`
	// FileExists checks that the given path exists relative to workdir.
	FileExists string `yaml:"file_exists,omitempty"`
	// Grep searches all .go files for the given pattern.
	Grep string `yaml:"grep,omitempty"`
	// TestExists searches _test.go files for a function with the given name.
	TestExists string `yaml:"test_exists,omitempty"`
}

// AssertionResult is the outcome of a single assertion check.
type AssertionResult struct {
	// Description is the human-readable label for this assertion.
	Description string `json:"description"`
	// Passed is true if the assertion succeeded.
	Passed bool `json:"passed"`
	// Detail provides extra context (e.g., matched file path or error message).
	Detail string `json:"detail,omitempty"`
}

// CheckpointStatus is the outcome of a single checkpoint's test command.
type CheckpointStatus struct {
	// Name is the checkpoint name.
	Name string `json:"name"`
	// Status is "pass", "fail", or "not_run".
	Status string `json:"status"`
	// Output is the combined stdout+stderr from the test command.
	Output string `json:"output,omitempty"`
}

// GateResult is the complete outcome of a gate run.
type GateResult struct {
	// Passed is true only if all assertions and checkpoints passed.
	Passed bool `json:"passed"`
	// Assertions holds per-assertion results.
	Assertions []AssertionResult `json:"assertions"`
	// Checkpoints holds per-checkpoint test results.
	Checkpoints []CheckpointStatus `json:"checkpoints"`
	// ScopedTestPassed is true if the phase's scoped test command succeeded.
	ScopedTestPassed bool `json:"scoped_test_passed"`
	// ScopedTestOutput is the combined output from the scoped test command.
	ScopedTestOutput string `json:"scoped_test_output,omitempty"`
	// ScopedTestSkipped is true when no scoped test command was configured.
	ScopedTestSkipped bool `json:"scoped_test_skipped"`
}

// GateStatus is the persistent state written to gate-status.json.
type GateStatus struct {
	// LastRun is the RFC3339 timestamp of the most recent gate execution.
	LastRun string `json:"last_run"`
	// RunCount is the number of times the gate has been run this session.
	RunCount int `json:"run_count"`
	// Passed mirrors the most recent GateResult.Passed value.
	Passed bool `json:"passed"`
	// Checkpoints maps checkpoint name → status string ("pass"/"fail"/"not_found").
	Checkpoints map[string]string `json:"checkpoints"`
}

// NewGateStatus creates a GateStatus initialized from a GateResult.
func NewGateStatus(result *GateResult) *GateStatus {
	cps := make(map[string]string, len(result.Checkpoints))
	for _, cp := range result.Checkpoints {
		cps[cp.Name] = cp.Status
	}
	return &GateStatus{
		LastRun:     time.Now().UTC().Format(time.RFC3339),
		RunCount:    1,
		Passed:      result.Passed,
		Checkpoints: cps,
	}
}
