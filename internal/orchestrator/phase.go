package orchestrator

import (
	"context"
	"log/slog"
)

// RunPhaseOptions configures execution of a single phase.
type RunPhaseOptions struct {
	PlanName  string
	PhaseName string
	PlansDir  string
	ArcHome   string
	Logger    *slog.Logger
}

// RunPhase executes a single phase from entry state to terminal state.
func RunPhase(ctx context.Context, opts RunPhaseOptions) error {
	panic("not implemented")
}
