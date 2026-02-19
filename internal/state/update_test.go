package state

import (
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

func TestSetStatus(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := SetStatus(sf, "implementing"); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus != "implementing" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "implementing")
	}
}

func TestSetStatusEmpty(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := SetStatus(sf, ""); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.PhaseStatus != "" {
		t.Fatalf("PhaseStatus = %q, want empty", got.PhaseStatus)
	}
}

func TestUpdateTestsStuckDetection(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	// Call UpdateTests with 3 passing, 3 times (same count each time)
	for i := 0; i < 3; i++ {
		if err := UpdateTests(sf, 3, 10); err != nil {
			t.Fatalf("UpdateTests call %d error: %v", i+1, err)
		}
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.StuckIterations != 3 {
		t.Fatalf("StuckIterations = %d, want 3", got.StuckIterations)
	}
}

func TestUpdateTestsProgressResetsStuck(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := UpdateTests(sf, 3, 10); err != nil {
		t.Fatalf("UpdateTests error: %v", err)
	}
	if err := UpdateTests(sf, 5, 10); err != nil {
		t.Fatalf("UpdateTests error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.StuckIterations != 0 {
		t.Fatalf("StuckIterations = %d, want 0", got.StuckIterations)
	}
}

func TestUpdateTestsZeroTotal(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := UpdateTests(sf, 0, 0); err != nil {
		t.Fatalf("UpdateTests error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.TestsPassing != 0 {
		t.Fatalf("TestsPassing = %d, want 0", got.TestsPassing)
	}
	if got.TestsTotal != 0 {
		t.Fatalf("TestsTotal = %d, want 0", got.TestsTotal)
	}
}

func TestFileDispute(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := FileDispute(sf, "test_foo", "conflicts with spec"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 1 {
		t.Fatalf("len(Disputes) = %d, want 1", len(got.Disputes))
	}
	if got.Disputes[0].TestName != "test_foo" {
		t.Fatalf("Disputes[0].TestName = %q, want %q", got.Disputes[0].TestName, "test_foo")
	}
	if got.PhaseStatus != "disputed" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "disputed")
	}
}

func TestFileDisputeNilResolution(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := FileDispute(sf, "test.go", "needs refactoring"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.Disputes[0].Resolution != nil {
		t.Fatalf("Disputes[0].Resolution = %v, want nil", got.Disputes[0].Resolution)
	}
}

func TestFileDisputeAppends(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := FileDispute(sf, "test_a", "r1"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}
	if err := FileDispute(sf, "test_b", "r2"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 2 {
		t.Fatalf("len(Disputes) = %d, want 2", len(got.Disputes))
	}
}

func TestRejectDispute(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := FileDispute(sf, "test_foo", "reason"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}
	if err := RejectDispute(sf, "test is correct"); err != nil {
		t.Fatalf("RejectDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 0 {
		t.Fatalf("len(Disputes) = %d, want 0", len(got.Disputes))
	}
	if got.PhaseStatus != "implementing" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "implementing")
	}
	if len(got.LastClearedDisputes) != 1 {
		t.Fatalf("len(LastClearedDisputes) = %d, want 1", len(got.LastClearedDisputes))
	}
}

func TestRejectDisputeEmptyDisputes(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := RejectDispute(sf, "reason"); err != nil {
		t.Fatalf("RejectDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 0 {
		t.Fatalf("len(Disputes) = %d, want 0", len(got.Disputes))
	}
	if len(got.LastClearedDisputes) != 0 {
		t.Fatalf("len(LastClearedDisputes) = %d, want 0", len(got.LastClearedDisputes))
	}
	if got.PhaseStatus != "implementing" {
		t.Fatalf("PhaseStatus = %q, want %q", got.PhaseStatus, "implementing")
	}
}

func TestApproveDispute(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := FileDispute(sf, "test_foo", "reason"); err != nil {
		t.Fatalf("FileDispute error: %v", err)
	}
	if err := ApproveDispute(sf, "fix the test"); err != nil {
		t.Fatalf("ApproveDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 1 {
		t.Fatalf("len(Disputes) = %d, want 1", len(got.Disputes))
	}
	if got.Disputes[0].Resolution == nil {
		t.Fatal("Disputes[0].Resolution = nil, want non-nil")
	}
	if *got.Disputes[0].Resolution != "approved" {
		t.Fatalf("Disputes[0].Resolution = %q, want %q", *got.Disputes[0].Resolution, "approved")
	}
}

func TestApproveDisputeEmptyDisputes(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := ApproveDispute(sf, "reason"); err != nil {
		t.Fatalf("ApproveDispute error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.Disputes) != 0 {
		t.Fatalf("len(Disputes) = %d, want 0", len(got.Disputes))
	}
}

func TestRecordVerdict(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := RecordVerdict(sf, "qa_review", arc.VerdictApproved); err != nil {
		t.Fatalf("RecordVerdict error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.VerdictsHistory) != 1 {
		t.Fatalf("len(VerdictsHistory) = %d, want 1", len(got.VerdictsHistory))
	}
	if got.VerdictsHistory[0].Verdict != "approved" {
		t.Fatalf("VerdictsHistory[0].Verdict = %q, want %q", got.VerdictsHistory[0].Verdict, "approved")
	}
	if got.VerdictsHistory[0].State != "qa_review" {
		t.Fatalf("VerdictsHistory[0].State = %q, want %q", got.VerdictsHistory[0].State, "qa_review")
	}
	if got.VerdictsHistory[0].Timestamp == "" {
		t.Fatal("VerdictsHistory[0].Timestamp is empty, want RFC3339 timestamp")
	}
	// Validate timestamp is RFC3339
	if _, parseErr := time.Parse(time.RFC3339, got.VerdictsHistory[0].Timestamp); parseErr != nil {
		t.Fatalf("Timestamp %q is not valid RFC3339: %v", got.VerdictsHistory[0].Timestamp, parseErr)
	}
	if got.LastVerdict != "approved" {
		t.Fatalf("LastVerdict = %q, want %q", got.LastVerdict, "approved")
	}
}

func TestRecordVerdictAccumulates(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := RecordVerdict(sf, "qa_review", arc.VerdictApproved); err != nil {
		t.Fatalf("RecordVerdict error: %v", err)
	}
	if err := RecordVerdict(sf, "impl_review", arc.VerdictConcerns); err != nil {
		t.Fatalf("RecordVerdict error: %v", err)
	}
	if err := RecordVerdict(sf, "qa_review", arc.VerdictGapsFound); err != nil {
		t.Fatalf("RecordVerdict error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.VerdictsHistory) != 3 {
		t.Fatalf("len(VerdictsHistory) = %d, want 3", len(got.VerdictsHistory))
	}
	if got.LastVerdict != "gaps_found" {
		t.Fatalf("LastVerdict = %q, want %q", got.LastVerdict, "gaps_found")
	}
}

func TestRecordVerdictIterationField(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	// Set iteration to 5
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.Iteration.Current = 5
		return nil
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	if err := RecordVerdict(sf, "qa_review", arc.VerdictApproved); err != nil {
		t.Fatalf("RecordVerdict error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.VerdictsHistory[0].Iteration != 5 {
		t.Fatalf("VerdictsHistory[0].Iteration = %d, want 5", got.VerdictsHistory[0].Iteration)
	}
}

func TestIncrementIteration(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := IncrementIteration(sf); err != nil {
		t.Fatalf("IncrementIteration error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.Iteration.Current != 1 {
		t.Fatalf("Iteration.Current = %d, want 1", got.Iteration.Current)
	}
}

func TestMarkReviewed(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	// Set iteration to 5
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.Iteration.Current = 5
		return nil
	}); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	if err := MarkReviewed(sf); err != nil {
		t.Fatalf("MarkReviewed error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.LastReviewedIter != 5 {
		t.Fatalf("LastReviewedIter = %d, want 5", got.LastReviewedIter)
	}
}

func TestMarkReviewedIterationZero(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := MarkReviewed(sf); err != nil {
		t.Fatalf("MarkReviewed error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.LastReviewedIter != 0 {
		t.Fatalf("LastReviewedIter = %d, want 0", got.LastReviewedIter)
	}
}

func TestAddTestFile(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.TestFiles) != 1 {
		t.Fatalf("len(TestFiles) = %d, want 1", len(got.TestFiles))
	}
	if got.TestFiles[0] != "tests/test_core.go" {
		t.Fatalf("TestFiles[0] = %q, want %q", got.TestFiles[0], "tests/test_core.go")
	}
}

func TestAddTestFileDedup(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}
	if err := AddTestFile(sf, "tests/test_core.go"); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(got.TestFiles) != 1 {
		t.Fatalf("len(TestFiles) = %d, want 1 (no duplicate)", len(got.TestFiles))
	}
}

func TestAddTestFileEmptyPath(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := AddTestFile(sf, ""); err != nil {
		t.Fatalf("AddTestFile error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	found := false
	for _, f := range got.TestFiles {
		if f == "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("TestFiles should contain empty string")
	}
}

func TestStartParallel(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results", []string{"branch_a", "branch_b"}); err != nil {
		t.Fatalf("StartParallel error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.ParallelExecution == nil {
		t.Fatal("ParallelExecution = nil, want non-nil")
	}
	if len(got.ParallelExecution.Branches) != 2 {
		t.Fatalf("len(Branches) = %d, want 2", len(got.ParallelExecution.Branches))
	}
	for _, name := range []string{"branch_a", "branch_b"} {
		b, ok := got.ParallelExecution.Branches[name]
		if !ok {
			t.Fatalf("branch %q not found", name)
		}
		if b.Status != "pending" {
			t.Fatalf("branch %q status = %q, want %q", name, b.Status, "pending")
		}
	}
	if got.ParallelExecution.StartedAt == "" {
		t.Fatal("StartedAt is empty, want RFC3339 timestamp")
	}
	if _, parseErr := time.Parse(time.RFC3339, got.ParallelExecution.StartedAt); parseErr != nil {
		t.Fatalf("StartedAt %q is not valid RFC3339: %v", got.ParallelExecution.StartedAt, parseErr)
	}
}

func TestStartParallelAlreadyActive(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results1", []string{"a"}); err != nil {
		t.Fatalf("first StartParallel error: %v", err)
	}
	if err := StartParallel(sf, "/tmp/results2", []string{"x", "y"}); err != nil {
		t.Fatalf("second StartParallel error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.ParallelExecution == nil {
		t.Fatal("ParallelExecution = nil, want non-nil")
	}
	// New session should replace old
	if got.ParallelExecution.ResultsDir != "/tmp/results2" {
		t.Fatalf("ResultsDir = %q, want %q", got.ParallelExecution.ResultsDir, "/tmp/results2")
	}
	if len(got.ParallelExecution.Branches) != 2 {
		t.Fatalf("len(Branches) = %d, want 2", len(got.ParallelExecution.Branches))
	}
}

func TestUpdateParallelBranch(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results", []string{"branch_a", "branch_b"}); err != nil {
		t.Fatalf("StartParallel error: %v", err)
	}
	if err := UpdateParallelBranch(sf, "branch_a", "passed", 0); err != nil {
		t.Fatalf("UpdateParallelBranch error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.ParallelExecution.Branches["branch_a"].Status != "passed" {
		t.Fatalf("branch_a status = %q, want %q", got.ParallelExecution.Branches["branch_a"].Status, "passed")
	}
}

func TestUpdateParallelBranchMissing(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results", []string{"branch_a"}); err != nil {
		t.Fatalf("StartParallel error: %v", err)
	}

	err := UpdateParallelBranch(sf, "nonexistent_branch", "passed", 0)
	if err == nil {
		t.Fatal("expected error for missing branch, got nil")
	}
	if !containsStr(err.Error(), "branch") || !containsStr(err.Error(), "not found") {
		t.Fatalf("error = %q, want containing 'branch' and 'not found'", err.Error())
	}
}

func TestUpdateParallelBranchNilParallel(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	err := UpdateParallelBranch(sf, "branch-a", "running", 0)
	if err == nil {
		t.Fatal("expected error for nil ParallelExecution, got nil")
	}
}

func TestFinishParallel(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results", []string{"branch_a", "branch_b"}); err != nil {
		t.Fatalf("StartParallel error: %v", err)
	}
	if err := UpdateParallelBranch(sf, "branch_a", "passed", 0); err != nil {
		t.Fatalf("UpdateParallelBranch error: %v", err)
	}
	if err := UpdateParallelBranch(sf, "branch_b", "passed", 0); err != nil {
		t.Fatalf("UpdateParallelBranch error: %v", err)
	}
	if err := FinishParallel(sf, "approved"); err != nil {
		t.Fatalf("FinishParallel error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got.ParallelExecution.Verdict != "approved" {
		t.Fatalf("Verdict = %q, want %q", got.ParallelExecution.Verdict, "approved")
	}
	if got.ParallelExecution.FinishedAt == "" {
		t.Fatal("FinishedAt is empty, want non-empty")
	}
}

func TestFinishParallelNil(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	err := FinishParallel(sf, "approved")
	if err == nil {
		t.Fatal("expected error for nil ParallelExecution, got nil")
	}
	if !containsStr(err.Error(), "no parallel execution") {
		t.Fatalf("error = %q, want containing 'no parallel execution'", err.Error())
	}
}

func TestFinishParallelHappyPathRFC3339(t *testing.T) {
	sf, _ := newTestStateFile(t)
	writeInitialState(t, sf)

	if err := StartParallel(sf, "/tmp/results", []string{"a"}); err != nil {
		t.Fatalf("StartParallel error: %v", err)
	}
	if err := FinishParallel(sf, "approved"); err != nil {
		t.Fatalf("FinishParallel error: %v", err)
	}

	got, err := sf.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if _, parseErr := time.Parse(time.RFC3339, got.ParallelExecution.FinishedAt); parseErr != nil {
		t.Fatalf("FinishedAt %q is not valid RFC3339: %v", got.ParallelExecution.FinishedAt, parseErr)
	}
}

// containsStr is a simple helper to avoid importing strings in this file.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
