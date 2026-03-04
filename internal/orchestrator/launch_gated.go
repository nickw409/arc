package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/gitops"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/worktree"
)

// LaunchGated starts the orchestrator for a gate-based plan.
// Each phase uses RunPhaseGated (session → gate → retry) instead of
// the workflow state machine.
//
// Phases are scheduled by dependency — independent phases run concurrently
// (up to Config.MaxParallel), each in its own worktree if UseWorktree is set.
func LaunchGated(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	if err := acquireLock(planDir); err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer releaseLock(planDir)

	// Apply wall-clock timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	// Load plan.json
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("reading plan.json: %w", err)
	}

	// Determine max parallelism
	maxParallel := 3
	if opts.Config != nil && opts.Config.MaxParallel > 0 {
		maxParallel = opts.Config.MaxParallel
	}

	// Shared worktree (when not using per-phase worktrees)
	var sharedWorktree *worktree.Worktree
	workingDir := ""
	if opts.UseWorktree && !opts.PerPhaseWorktree {
		projectDir := opts.ProjectDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}
		wt, wtErr := worktree.Create(projectDir, opts.PlanName, "")
		if wtErr != nil {
			opts.Logger.Warn("failed to create shared worktree, running in-tree", "error", wtErr)
		} else {
			sharedWorktree = wt
			workingDir = wt.Dir
		}
	}

	// Print header
	fmt.Println("==========================================")
	fmt.Println("  Arc Orchestrator (gate-based)")
	fmt.Println("==========================================")
	fmt.Printf("Plan: %s\n", opts.PlanName)
	fmt.Printf("Phases: %s\n", strings.Join(meta.Phases, ", "))
	fmt.Printf("Max parallel: %d\n", maxParallel)
	if opts.Timeout > 0 {
		fmt.Printf("Timeout: %ds\n", opts.Timeout)
	}
	fmt.Println("==========================================")
	fmt.Println()

	buildResult := func(status, failedPhase, failedReason string) *LaunchResult {
		phaseStates := loadAllPhaseStates(planDir, meta.Phases)
		summary := make(map[string]string, len(meta.Phases))
		var totalUsage arc.Usage
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps == nil {
				summary[p] = "pending"
				continue
			}
			summary[p] = ps.PhaseStatus
			totalUsage = totalUsage.Add(ps.Usage)
		}
		return &LaunchResult{
			Status:       status,
			FailedPhase:  failedPhase,
			FailedReason: failedReason,
			PhaseSummary: summary,
			Usage:        totalUsage,
		}
	}

	// Track running phases
	running := make(map[string]bool)

	// Scheduling loop — dependency-driven
	for {
		if ctx.Err() != nil {
			return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
		}

		phaseStates := loadAllPhaseStates(planDir, meta.Phases)

		// Check if all done
		allDone := true
		for _, phase := range meta.Phases {
			ps := phaseStates[phase]
			if ps == nil {
				allDone = false
				continue
			}
			status := ps.PhaseStatus
			if status != "complete" && status != "blocked" && status != "deferred" {
				allDone = false
			}
		}

		if allDone {
			return gatedPlanComplete(opts, meta, planDir, sharedWorktree, phaseStates, buildResult)
		}

		// Find ready phases
		ready := state.PhasesReady(meta, phaseStates)
		var toRun []string
		for _, phase := range ready {
			if !running[phase] {
				toRun = append(toRun, phase)
			}
		}

		if len(toRun) == 0 && len(running) == 0 {
			// No phases ready and none running — stuck
			fmt.Println("\nNo phases ready to execute.")
			printBlockedSummary(meta, phaseStates)
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		if len(toRun) == 0 {
			// Phases are running but none ready to launch — wait for results
			// This shouldn't happen in the current architecture since we collect
			// all results below before looping, but guard against it.
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		// Limit parallelism
		if len(toRun) > maxParallel {
			toRun = toRun[:maxParallel]
		}

		// Launch phases concurrently
		type phaseResult struct {
			phase string
			err   error
		}
		results := make(chan phaseResult, len(toRun))
		var wg sync.WaitGroup

		batchCtx, batchCancel := context.WithCancel(ctx)

		for _, phase := range toRun {
			running[phase] = true
			wg.Add(1)
			go func(phaseName string) {
				defer wg.Done()
				opts.Logger.Info("starting phase", "phase", phaseName)
				fmt.Printf("\n[%s] Starting phase\n", phaseName)

				err := RunPhaseGated(batchCtx, RunPhaseOptions{
					PlanName:    opts.PlanName,
					PhaseName:   phaseName,
					PlansDir:    opts.PlansDir,
					ArcHome:     opts.ArcHome,
					ProjectDir:  opts.ProjectDir,
					Config:      opts.Config,
					Logger:      opts.Logger,
					UseWorktree: opts.UseWorktree && opts.PerPhaseWorktree,
					WorkingDir:  workingDir,
					ChatMode:    opts.ChatMode,
					Resolver:    opts.Resolver,
				})
				results <- phaseResult{phase: phaseName, err: err}
			}(phase)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		var fatalErr error
		var failedPhase string
		for r := range results {
			delete(running, r.phase)
			if r.err != nil {
				if ctx.Err() != nil {
					batchCancel()
					return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
				}
				ps := loadPhaseState(planDir, r.phase)
				if ps != nil && (ps.PhaseStatus == "blocked" || ps.PhaseStatus == "deferred") {
					if opts.StopOnFailure {
						if fatalErr == nil {
							fatalErr = fmt.Errorf("phase %s %s: %w", r.phase, ps.PhaseStatus, r.err)
							failedPhase = r.phase
							batchCancel()
						}
					} else {
						fmt.Printf("[%s] Phase %s, continuing...\n", r.phase, ps.PhaseStatus)
					}
					continue
				}
				if fatalErr == nil {
					fatalErr = fmt.Errorf("phase %s failed: %w", r.phase, r.err)
					failedPhase = r.phase
					if opts.StopOnFailure {
						batchCancel()
					}
				}
			} else {
				fmt.Printf("[%s] Complete\n", r.phase)
			}
		}
		batchCancel()

		if fatalErr != nil {
			if opts.StopOnFailure {
				return buildResult("failed", failedPhase, fatalErr.Error()), nil
			}
			// Continue to next loop iteration — other phases might be ready
		}
	}
}

// gatedPlanComplete handles the success path for a gate-based plan:
// merge worktree, generate report.
func gatedPlanComplete(
	opts LaunchOptions,
	meta *arc.PlanMeta,
	planDir string,
	sharedWorktree *worktree.Worktree,
	phaseStates map[string]*arc.PhaseState,
	buildResult func(string, string, string) *LaunchResult,
) (*LaunchResult, error) {
	fmt.Println("\nAll phases complete.")

	// Merge shared worktree
	if sharedWorktree != nil {
		commitMsg := fmt.Sprintf("feat(%s): phase work", opts.PlanName)
		if hash, commitErr := gitops.Commit(gitops.CommitOptions{
			Message: commitMsg,
			Dir:     sharedWorktree.Dir,
			Config:  opts.Config,
		}); commitErr != nil {
			opts.Logger.Warn("pre-merge commit failed", "error", commitErr)
		} else if hash != "" {
			fmt.Printf("Committed worktree changes: %s\n", shortHash(hash))
		}

		if hash, mergeErr := worktree.MergeBack(sharedWorktree); mergeErr != nil {
			opts.Logger.Warn("worktree merge failed", "branch", sharedWorktree.Branch, "error", mergeErr)
			return buildResult("failed", "", fmt.Sprintf("worktree merge failed: %v", mergeErr)),
				fmt.Errorf("worktree merge failed: %w", mergeErr)
		} else {
			fmt.Printf("Merged worktree: %s\n", shortHash(hash))
			worktree.Remove(sharedWorktree)
		}
	}

	// Generate reports
	if err := generateCompletionReport(planDir, opts.PlanName, meta, phaseStates); err != nil {
		return nil, err
	}
	if _, err := plan.GenerateSummary(plan.SummaryOptions{
		PlanDir:     planDir,
		PlanName:    opts.PlanName,
		Meta:        meta,
		PhaseStates: phaseStates,
		ProjectDir:  opts.ProjectDir,
	}); err != nil {
		opts.Logger.Warn("failed to generate summary", "error", err)
	}

	return buildResult("complete", "", ""), nil
}
