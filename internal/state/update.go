package state

import "github.com/nwiley/arc/internal/arc"

// SetStatus updates phase_status to the given value.
func SetStatus(sf *StateFile, status string) error {
	panic("not implemented")
}

// FileDispute adds a new dispute to the disputes list and sets status to "disputed".
func FileDispute(sf *StateFile, testName, reason string) error {
	panic("not implemented")
}

// RejectDispute clears ALL disputes from the list, sets status to "implementing",
// and moves cleared disputes to last_cleared_disputes for audit.
func RejectDispute(sf *StateFile, reason string) error {
	panic("not implemented")
}

// ApproveDispute marks ALL current disputes with resolution "approved".
func ApproveDispute(sf *StateFile, reason string) error {
	panic("not implemented")
}

// UpdateTests updates test counts and stuck_iterations tracking.
func UpdateTests(sf *StateFile, passing, total int) error {
	panic("not implemented")
}

// IncrementIteration bumps iteration.current by 1.
func IncrementIteration(sf *StateFile) error {
	panic("not implemented")
}

// MarkReviewed sets last_reviewed_iteration to the current iteration.current value.
func MarkReviewed(sf *StateFile) error {
	panic("not implemented")
}

// AddTestFile appends a test file path to test_files if not already present.
func AddTestFile(sf *StateFile, path string) error {
	panic("not implemented")
}

// RecordVerdict appends to verdicts_history and sets last_verdict.
func RecordVerdict(sf *StateFile, stateName string, verdict arc.Verdict) error {
	panic("not implemented")
}

// StartParallel initializes parallel execution tracking.
func StartParallel(sf *StateFile, resultsDir string, branches []string) error {
	panic("not implemented")
}

// UpdateParallelBranch updates a single branch's status and exit code.
func UpdateParallelBranch(sf *StateFile, branch, status string, exitCode int) error {
	panic("not implemented")
}

// FinishParallel sets the final verdict and finished_at timestamp on parallel execution.
func FinishParallel(sf *StateFile, verdict string) error {
	panic("not implemented")
}
