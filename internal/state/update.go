package state

import (
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

// UpdateTests updates test counts.
func UpdateTests(sf *StateFile, passing, total int) error {
	return sf.Update(func(s *arc.PhaseState) error {
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
