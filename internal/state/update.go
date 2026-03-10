package state

import (
	"fmt"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// SetStatus updates phase_status to the given value.
func SetStatus(sf *StateFile, status string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = status
		return nil
	})
}

// FileDispute adds a new dispute to the disputes list and sets status to "disputed".
func FileDispute(sf *StateFile, testName, reason string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.Disputes = append(s.Disputes, arc.Dispute{
			TestName: testName,
			Reason:   reason,
		})
		s.PhaseStatus = "disputed"
		return nil
	})
}

// RejectDispute clears ALL disputes from the list, sets status to "implementing",
// and moves cleared disputes to last_cleared_disputes for audit.
func RejectDispute(sf *StateFile, reason string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		for i := range s.Disputes {
			s.Disputes[i].ResolutionReason = reason
		}
		s.LastClearedDisputes = s.Disputes
		s.Disputes = []arc.Dispute{}
		s.PhaseStatus = "implementing"
		return nil
	})
}

// ApproveDispute marks ALL current disputes with resolution "approved".
func ApproveDispute(sf *StateFile, reason string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		approved := "approved"
		for i := range s.Disputes {
			s.Disputes[i].Resolution = &approved
			s.Disputes[i].ResolutionReason = reason
		}
		return nil
	})
}

// SetActivity sets the agent's current activity message and timestamp.
// Passing an empty string clears the activity.
func SetActivity(sf *StateFile, activity string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.Activity = activity
		if activity == "" {
			s.ActivityUpdatedAt = ""
		} else {
			s.ActivityUpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		return nil
	})
}

// UpdateTests updates test counts and stuck_iterations tracking.
func UpdateTests(sf *StateFile, passing, total int) error {
	return sf.Update(func(s *arc.PhaseState) error {
		if passing > s.TestsPassing && s.TestsPassing > 0 {
			s.StuckIterations = 0
		} else {
			s.StuckIterations++
		}
		s.TestsPassing = passing
		s.TestsTotal = total
		return nil
	})
}

// IncrementIteration bumps iteration.current by 1.
func IncrementIteration(sf *StateFile) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.Iteration.Current++
		return nil
	})
}

// MarkReviewed sets last_reviewed_iteration to the current iteration.current value.
func MarkReviewed(sf *StateFile) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.LastReviewedIter = s.Iteration.Current
		return nil
	})
}

// AddTestFile appends a test file path to test_files if not already present.
func AddTestFile(sf *StateFile, path string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		for _, f := range s.TestFiles {
			if f == path {
				return nil
			}
		}
		s.TestFiles = append(s.TestFiles, path)
		return nil
	})
}

// RecordVerdict appends to verdicts_history and sets last_verdict.
func RecordVerdict(sf *StateFile, stateName string, verdict arc.Verdict) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.VerdictsHistory = append(s.VerdictsHistory, arc.VerdictEntry{
			Iteration: s.Iteration.Current,
			State:     stateName,
			Verdict:   string(verdict),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		s.LastVerdict = string(verdict)
		return nil
	})
}

// StartParallel initializes parallel execution tracking.
func StartParallel(sf *StateFile, resultsDir string, branches []string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		branchMap := make(map[string]arc.BranchStatus, len(branches))
		for _, b := range branches {
			branchMap[b] = arc.BranchStatus{Status: "pending"}
		}
		s.ParallelExecution = &arc.ParallelExec{
			ResultsDir: resultsDir,
			Branches:   branchMap,
			StartedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		return nil
	})
}

// UpdateParallelBranch updates a single branch's status and exit code.
func UpdateParallelBranch(sf *StateFile, branch, status string, exitCode int) error {
	return sf.Update(func(s *arc.PhaseState) error {
		if s.ParallelExecution == nil {
			return fmt.Errorf("no parallel execution active")
		}
		if _, ok := s.ParallelExecution.Branches[branch]; !ok {
			return fmt.Errorf("branch %q not found", branch)
		}
		s.ParallelExecution.Branches[branch] = arc.BranchStatus{
			Status:   status,
			ExitCode: exitCode,
		}
		return nil
	})
}

// IncrementWatchAttempts increments the watch attempt counter for a phase.
// Called before spawning an intervention agent so crashes still count as attempts.
func IncrementWatchAttempts(sf *StateFile) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.WatchAttempts++
		return nil
	})
}

// ResetToRetry resets a blocked phase to pending so the orchestrator can retry it.
// Called after a watch intervention agent exits.
func ResetToRetry(sf *StateFile) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "pending"
		s.BlockedReason = ""
		s.BlockedAt = ""
		return nil
	})
}

// AppendAttemptLog appends a gate attempt summary to the phase's attempt_log.
func AppendAttemptLog(sf *StateFile, summary arc.AttemptSummary) error {
	return sf.Update(func(s *arc.PhaseState) error {
		s.AttemptLog = append(s.AttemptLog, summary)
		return nil
	})
}

// FinishParallel sets the final verdict and finished_at timestamp on parallel execution.
func FinishParallel(sf *StateFile, verdict string) error {
	return sf.Update(func(s *arc.PhaseState) error {
		if s.ParallelExecution == nil {
			return fmt.Errorf("no parallel execution active")
		}
		s.ParallelExecution.Verdict = verdict
		s.ParallelExecution.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
}
