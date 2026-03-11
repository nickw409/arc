package arc

// GateAssertion defines a single verifiable condition for a phase gate.
type GateAssertion struct {
	// Type is the assertion kind: "file_exists", "grep", "test_exists",
	// "build_passes", or "no_untracked".
	Type string `json:"type,omitempty" yaml:"type"`
	// Description is a human-readable label for this assertion.
	Description string `json:"description,omitempty" yaml:"description"`
	// Target is the file path (for file_exists), pattern (for grep),
	// function name (for test_exists), or build command (for build_passes).
	Target string `json:"target,omitempty" yaml:"target"`
	// FileExists checks that the given path exists relative to workdir.
	FileExists string `json:"file_exists,omitempty" yaml:"file_exists,omitempty"`
	// Grep searches all .go files for the given pattern.
	Grep string `json:"grep,omitempty" yaml:"grep,omitempty"`
	// TestExists searches _test.go files for a function with the given name.
	TestExists string `json:"test_exists,omitempty" yaml:"test_exists,omitempty"`
	// BuildPasses runs the given build command and checks for exit code 0.
	BuildPasses string `json:"build_passes,omitempty" yaml:"build_passes,omitempty"`
	// NoUntracked checks that no debug/temp artifact files are untracked in git.
	// The value is ignored; any non-empty string enables this assertion.
	NoUntracked string `json:"no_untracked,omitempty" yaml:"no_untracked,omitempty"`
	// FileAbsent checks that the given path does NOT exist relative to workdir.
	FileAbsent string `json:"file_absent,omitempty" yaml:"file_absent,omitempty"`
	// GrepNot searches all .go files in workdir and fails if the pattern IS found.
	// Complement of Grep.
	GrepNot string `json:"grep_not,omitempty" yaml:"grep_not,omitempty"`
	// NoModified checks that the given path (relative to workdir) has no uncommitted
	// changes according to git diff HEAD. Fails if the file was modified.
	NoModified string `json:"no_modified,omitempty" yaml:"no_modified,omitempty"`
	// FilesOnly checks that every file changed (git diff HEAD --name-only) matches
	// at least one of the comma-separated glob patterns. Fails if any changed file
	// falls outside the allowed set.
	FilesOnly string `json:"files_only,omitempty" yaml:"files_only,omitempty"`
	// SpecCoverage checks that the given symbol or pattern appears in at least one
	// _test.go file under workdir. Non-empty value enables this assertion.
	SpecCoverage string `json:"spec_coverage,omitempty" yaml:"spec_coverage,omitempty"`
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
	// TestCoversQueue holds coverage targets collected from promise test_covers fields.
	// These are queued for coverage verification in subsequent phases.
	TestCoversQueue []string `json:"test_covers_queue,omitempty"`
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

