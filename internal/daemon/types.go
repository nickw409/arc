package daemon

import (
	"context"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/worktree"
)

// PlanRegistration holds the full per-plan state within the daemon.
type PlanRegistration struct {
	// Identity
	PlanName   string
	ProjectDir string
	PlansDir   string
	ArcHome    string

	// Meta
	Meta *arc.PlanMeta

	// Options
	Timeout          int
	UseWorktree      bool
	PerPhaseWorktree bool
	StopOnFailure    bool
	ChatMode         bool
	ConfigPath       string

	// Runtime (not persisted)
	Config     *config.Config
	Resolver   *resources.Resolver
	Worktree   *worktree.Worktree
	PlanLogger *orchestrator.PlanLogger

	// Scheduling
	SubmittedAt     time.Time
	PhaseStates     map[string]*arc.PhaseState
	PendingFinalize bool

	// Context (not persisted)
	Ctx    context.Context
	Cancel context.CancelFunc
}

// PhaseResult is emitted when a phase completes (success or failure).
type PhaseResult struct {
	PlanName  string
	PhaseName string
	Err       error
	Finalize  bool
}

// WorkItem represents a unit of work for the scheduler.
type WorkItem struct {
	PlanName     string
	PhaseName    string
	IsFinalize   bool
	Registration *PlanRegistration
}
