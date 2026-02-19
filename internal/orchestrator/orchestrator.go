package orchestrator

import (
	"context"
	"log/slog"
)

// LaunchOptions configures the orchestrator launcher.
type LaunchOptions struct {
	PlanName string
	PlansDir string
	ArcHome  string
	Logger   *slog.Logger
}

// Launch starts the orchestrator for a plan.
func Launch(ctx context.Context, opts LaunchOptions) error {
	panic("not implemented")
}

// acquireLock creates <planDir>/.orchestrator.lock with current PID.
func acquireLock(planDir string) error {
	panic("not implemented")
}

// releaseLock removes <planDir>/.orchestrator.lock.
func releaseLock(planDir string) {
	panic("not implemented")
}
