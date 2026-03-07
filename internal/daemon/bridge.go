package daemon

import (
	"context"
	"log/slog"

	"github.com/nwiley/arc/internal/orchestrator"
)

// BuildPhaseOptions constructs RunPhaseOptions from a PlanRegistration for a given phase.
func BuildPhaseOptions(reg *PlanRegistration, phaseName string) orchestrator.RunPhaseOptions {
	opts := orchestrator.RunPhaseOptions{
		PlanName:   reg.PlanName,
		PhaseName:  phaseName,
		PlansDir:   reg.PlansDir,
		ArcHome:    reg.ArcHome,
		ProjectDir: reg.ProjectDir,
		Config:     reg.Config,
		Logger:     slog.Default(),
		ChatMode:   reg.ChatMode,
		Resolver:   reg.Resolver,
		PlanLogger: reg.PlanLogger,
	}

	if reg.PerPhaseWorktree {
		opts.UseWorktree = true
	} else if reg.Worktree != nil {
		opts.WorkingDir = reg.Worktree.Dir
	}

	return opts
}

// BuildLaunchOptions constructs LaunchOptions from a PlanRegistration for finalize.
func BuildLaunchOptions(reg *PlanRegistration) orchestrator.LaunchOptions {
	return orchestrator.LaunchOptions{
		PlanName:         reg.PlanName,
		PlansDir:         reg.PlansDir,
		ArcHome:          reg.ArcHome,
		ProjectDir:       reg.ProjectDir,
		Config:           reg.Config,
		ConfigPath:       reg.ConfigPath,
		Logger:           slog.Default(),
		Timeout:          reg.Timeout,
		UseWorktree:      reg.UseWorktree,
		PerPhaseWorktree: reg.PerPhaseWorktree,
		StopOnFailure:    reg.StopOnFailure,
		ChatMode:         reg.ChatMode,
		Resolver:         reg.Resolver,
		PlanLogger:       reg.PlanLogger,
	}
}

// DefaultPhaseRunner returns a PhaseRunner that calls orchestrator.RunPhaseGated.
// sched may be nil (e.g. in tests); if non-nil its RateLimitSignal is wired in.
func DefaultPhaseRunner(sched *Scheduler) PhaseRunner {
	return func(ctx context.Context, reg *PlanRegistration, phaseName string) error {
		opts := BuildPhaseOptions(reg, phaseName)
		if sched != nil {
			opts.OnRateLimit = sched.RateLimitSignal
		}
		return orchestrator.RunPhaseGated(ctx, opts)
	}
}

// DefaultFinalizer returns a Finalizer that calls orchestrator.GatedPlanComplete.
func DefaultFinalizer() Finalizer {
	return func(reg *PlanRegistration) {
		opts := BuildLaunchOptions(reg)
		pd := planDir(reg)
		orchestrator.GatedPlanComplete(opts, reg.Meta, pd, reg.Worktree, reg.PhaseStates)
	}
}
