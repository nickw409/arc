package orchestrator

import "github.com/nwiley/arc/internal/arc"

// ErrorTier represents the severity and retry strategy for a failure.
type ErrorTier int

const (
	// TierTransient — rate limit, timeout, crash with no output. Auto-retry.
	TierTransient ErrorTier = 1
	// TierFeedback — gate failed with actionable feedback. Retry with context.
	TierFeedback ErrorTier = 2
	// TierStrategic — multiple retries with no progress. Needs orchestrator agent.
	TierStrategic ErrorTier = 3
	// TierGiveUp — exhausted all options.
	TierGiveUp ErrorTier = 4
)

// classifySpawnError determines the retry tier for an agent spawn failure.
func classifySpawnError(result *arc.AgentResult, err error) ErrorTier {
	if result == nil {
		return TierTransient
	}
	if result.TimedOut || result.InactivityKill {
		return TierTransient
	}
	// Non-zero exit with no output = crash
	if result.ExitCode != 0 && result.Output == "" {
		return TierTransient
	}
	// Agent produced output but exited non-zero — partial progress, try gate
	return TierFeedback
}

// classifyGateFailure determines the retry tier based on gate results and attempt history.
// prevPassed is the number of checkpoints that passed on the previous attempt (0 if first).
func classifyGateFailure(result *arc.GateResult, attempt, maxAttempts, prevPassed int) ErrorTier {
	if attempt >= maxAttempts {
		return TierGiveUp
	}

	passed := countCheckpointsPassed(result)

	// No progress after multiple attempts — strategic intervention needed
	if attempt >= 2 && passed <= prevPassed && passed == 0 {
		return TierStrategic
	}

	return TierFeedback
}

// countCheckpointsPassed returns how many checkpoints passed in a gate result.
func countCheckpointsPassed(result *arc.GateResult) int {
	if result == nil {
		return 0
	}
	count := 0
	for _, cp := range result.Checkpoints {
		if cp.Status == "pass" {
			count++
		}
	}
	return count
}
