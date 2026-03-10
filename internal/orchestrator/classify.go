package orchestrator

import "github.com/nwiley/arc/internal/arc"

// ErrorTier represents the severity and retry strategy for a failure.
type ErrorTier int

const (
	// TierTransient — rate limit, timeout, crash with no output. Auto-retry.
	TierTransient ErrorTier = 1
	// TierFeedback — gate failed with actionable feedback. Retry with context.
	TierFeedback ErrorTier = 2
	// TierGiveUp — exhausted all options.
	TierGiveUp ErrorTier = 3
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
func classifyGateFailure(result *arc.GateResult, attempt, maxAttempts int) ErrorTier {
	if attempt >= maxAttempts {
		return TierGiveUp
	}
	return TierFeedback
}
