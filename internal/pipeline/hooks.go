package pipeline

import (
	"context"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// RunAfterHooks executes after-hooks for a state, filtered by verdict.
func RunAfterHooks(ctx context.Context, hooks []arc.HookConfig, verdict arc.Verdict, state *arc.PhaseState, phaseDir string) error {
	var firstErr error
	for _, hook := range hooks {
		if !hookMatches(hook.When, verdict) {
			continue
		}
		actx := ActionContext{
			PhaseDir: phaseDir,
			State:    state,
		}
		if err := RunAction(ctx, hook.Action, hook.Params, actx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func hookMatches(when string, verdict arc.Verdict) bool {
	verdictStr := string(verdict)

	// Empty when: always runs
	if when == "" {
		return true
	}

	// Contains "|": split and check each segment (OR logic)
	if strings.Contains(when, "|") {
		segments := strings.Split(when, "|")
		for _, seg := range segments {
			if matchSegment(seg, verdictStr) {
				return true
			}
		}
		return false
	}

	// Single segment
	return matchSegment(when, verdictStr)
}

func matchSegment(seg, verdictStr string) bool {
	if strings.HasPrefix(seg, "!") {
		// NOT match: verdict != remainder
		remainder := seg[1:]
		return verdictStr != remainder
	}
	// Exact match
	return seg == verdictStr
}
