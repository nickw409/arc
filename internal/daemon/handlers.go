package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/worktree"
)

// HandleConnection reads a single request from conn, dispatches it, and writes
// the response. It is intended to be called in a goroutine per connection.
func HandleConnection(conn net.Conn, sched *Scheduler, cfg *DaemonConfig) {
	defer conn.Close()

	var req Request
	if err := ReadMessage(conn, &req); err != nil {
		_ = WriteMessage(conn, Response{OK: false, Error: fmt.Sprintf("decoding request: %v", err)})
		return
	}

	var resp Response
	switch req.Cmd {
	case "submit":
		resp = handleSubmit(req, sched, cfg)
	case "status":
		if req.Plan != "" {
			resp = *sched.Status(req.Plan)
		} else {
			resp = *sched.GlobalStatus()
		}
	case "cancel":
		if err := sched.Cancel(req.Plan); err != nil {
			resp = Response{OK: false, Error: err.Error()}
		} else {
			resp = Response{OK: true}
		}
	case "drain":
		sched.Drain()
		resp = Response{OK: true}
	case "list":
		resp = handleList(sched)
	case "sync":
		if err := sched.Sync(req.Plan); err != nil {
			resp = Response{OK: false, Error: err.Error()}
		} else {
			resp = Response{OK: true}
		}
	default:
		resp = Response{OK: false, Error: fmt.Sprintf("unknown command: %q", req.Cmd)}
	}

	_ = WriteMessage(conn, resp)
}

func handleList(sched *Scheduler) Response {
	plans := sched.ListPlans()
	return Response{OK: true, ActivePlans: plans}
}

func handleSubmit(req Request, sched *Scheduler, cfg *DaemonConfig) Response {
	// Resolve paths.
	plansDir := filepath.Join(req.Project, ".plans", "active")
	planDir := filepath.Join(plansDir, req.Plan)

	// Validate plan exists.
	if _, err := os.Stat(planDir); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("plan not found: %v", err)}
	}

	// Load meta and check review status.
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("reading plan: %v", err)}
	}
	if meta.ReviewStatus != "approved" && meta.ReviewStatus != "conditional" {
		return Response{OK: false, Error: fmt.Sprintf("plan review status is %q (must be approved or conditional)", meta.ReviewStatus)}
	}

	// Acquire per-plan lock.
	if err := orchestrator.AcquirePlanLock(planDir, os.Getpid()); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("acquiring plan lock: %v", err)}
	}

	// From here, we must release the lock on error.
	releaseLock := func() {
		orchestrator.ReleasePlanLock(planDir)
	}

	// Load project config.
	projCfg, err := config.Load(req.Project)
	if err != nil {
		releaseLock()
		return Response{OK: false, Error: fmt.Sprintf("loading config: %v", err)}
	}

	// Create resolver.
	homeDir, _ := os.UserHomeDir()
	resolver := resources.NewResolver(req.Project, homeDir)

	// Create shared worktree if requested (not per-phase).
	var wt *worktree.Worktree
	if req.UseWorktree && !req.PerPhaseWorktree {
		created, wtErr := worktree.Create(req.Project, req.Plan, "", projCfg.Git.BaseBranch)
		if wtErr != nil {
			slog.Warn("failed to create worktree, running in-tree", "error", wtErr)
		} else {
			wt = created
		}
	}

	// Create plan logger.
	planLogger := orchestrator.NewPlanLogger(planDir, slog.Default())

	// Load all phase states.
	phaseStates := orchestrator.LoadAllPhaseStates(planDir, meta.Phases)

	// Create context with timeout.
	ctx := context.Background()
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	reg := &PlanRegistration{
		PlanName:         req.Plan,
		ProjectDir:       req.Project,
		PlansDir:         plansDir,
		ArcHome:          homeDir,
		Meta:             meta,
		Timeout:          req.Timeout,
		UseWorktree:      req.UseWorktree,
		PerPhaseWorktree: req.PerPhaseWorktree,
		StopOnFailure:    req.StopOnFailure,
		ChatMode:         req.ChatMode,
		ConfigPath:       req.ConfigPath,
		Config:           projCfg,
		Resolver:         resolver,
		Worktree:         wt,
		PlanLogger:       planLogger,
		SubmittedAt:      time.Now(),
		PhaseStates:      phaseStates,
		Ctx:              ctx,
		Cancel:           cancel,
	}

	queued := len(state.PhasesReady(meta, phaseStates))
	if queued == 0 {
		cancel()
		releaseLock()
		return Response{OK: false, Error: "plan has no pending phases to run (all phases are complete, blocked, or blocked by a failed dependency)"}
	}

	if err := sched.Register(reg); err != nil {
		cancel()
		releaseLock()
		if wt != nil {
			_ = worktree.Remove(wt)
		}
		return Response{OK: false, Error: fmt.Sprintf("registering plan: %v", err)}
	}

	return Response{OK: true, QueuedPhases: queued}
}
