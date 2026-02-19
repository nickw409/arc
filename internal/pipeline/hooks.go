package pipeline

import (
	"context"

	"github.com/nwiley/arc/internal/arc"
)

// RunAfterHooks executes after-hooks for a state, filtered by verdict.
func RunAfterHooks(ctx context.Context, hooks []arc.HookConfig, verdict arc.Verdict, state *arc.PhaseState, phaseDir string) error {
	panic("not implemented")
}
