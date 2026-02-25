package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/worktree"
)

// LaunchOptions configures the orchestrator launcher.
type LaunchOptions struct {
	PlanName      string
	PlansDir      string
	ArcHome       string
	ProjectDir    string // working directory for git commits; empty uses process cwd
	Config        *config.Config
	Logger        *slog.Logger
	Timeout       int  // wall-clock timeout in seconds (0 = no timeout)
	UseWorktree      bool // if true, run agents in isolated git worktrees
	PerPhaseWorktree bool // if true, create a separate worktree per phase instead of one shared worktree
	StopOnFailure    bool // if true, cancel in-progress phases and return on first failure
	ChatMode      bool // if true, skip escalation ladder and block immediately for chat-agent intervention
}

// LaunchResult describes the outcome of an orchestrator run.
type LaunchResult struct {
	Status       string            // "complete", "failed", "cancelled", "blocked"
	FailedPhase  string            // which phase caused the stop (empty if complete)
	FailedReason string            // why it failed
	PhaseSummary map[string]string // phase name → final status
	Usage        arc.Usage
}

// Launch starts the orchestrator for a plan.
func Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
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

	// Clean up review output files from previous adversarial reviews
	if err := review.CleanupOutputFiles(planDir, meta.Phases); err != nil {
		opts.Logger.Warn("failed to clean review output files", "error", err)
	}

	// Set up shared worktree for all phases (unless per-phase worktrees are requested)
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
			defer func() {
				if sharedWorktree != nil {
					worktree.Remove(sharedWorktree)
				}
			}()
		}
	}

	// Print header
	fmt.Println("==========================================")
	fmt.Println("  Arc Orchestrator")
	fmt.Println("==========================================")
	fmt.Printf("Plan: %s\n", opts.PlanName)
	fmt.Printf("Phases: %s\n", strings.Join(meta.Phases, ", "))
	if opts.Timeout > 0 {
		fmt.Printf("Timeout: %ds (%dh %dm)\n", opts.Timeout, opts.Timeout/3600, (opts.Timeout%3600)/60)
	}
	fmt.Println("==========================================")
	fmt.Println()

	// Helper to build a LaunchResult from current phase states.
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

	// Track phases already running to avoid double-launching.
	running := make(map[string]bool)

	// Main orchestration loop
	for {
		if ctx.Err() != nil {
			fmt.Println("\nOrchestrator timed out or cancelled.")
			fmt.Println("Re-run to continue from where it left off.")
			return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
		}

		// Load all phase states
		phaseStates := loadAllPhaseStates(planDir, meta.Phases)

		// Check if all phases are done
		allDone := true
		for _, phase := range meta.Phases {
			ps := phaseStates[phase]
			if ps == nil {
				allDone = false
				continue
			}
			status := ps.PhaseStatus
			if status != "complete" && status != "blocked" && status != "split" && status != "deferred" {
				allDone = false
			}
		}

		if allDone {
			fmt.Println("\nAll phases complete.")

			// Merge shared worktree back before reporting completion
			if sharedWorktree != nil {
				if hash, mergeErr := worktree.MergeBack(sharedWorktree); mergeErr != nil {
					opts.Logger.Warn("shared worktree merge failed, preserving branch for manual resolution", "branch", sharedWorktree.Branch, "error", mergeErr)
					sharedWorktree = nil // prevent deferred Remove from deleting the branch
					return buildResult("failed", "", fmt.Sprintf("worktree merge failed: %v", mergeErr)), fmt.Errorf("worktree merge failed: %w", mergeErr)
				} else {
					fmt.Printf("Merged worktree branch %s: %s\n", sharedWorktree.Branch, hash[:7])
				}
			}

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
			if meta.WorkflowType == "performance" {
				printUsageSummary(meta, phaseStates)
			}
			return buildResult("complete", "", ""), nil
		}

		// Find all ready phases
		ready := state.PhasesReady(meta, phaseStates)
		if len(ready) == 0 {
			// No phases are ready — all remaining are blocked by dependencies
			fmt.Println("\nNo phases ready to execute. Remaining phases are blocked by dependencies.")
			printBlockedSummary(meta, phaseStates)
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		// Filter out phases that are already being tracked as running
		// (shouldn't happen in the current flow, but guard against it).
		var toRun []string
		for _, phase := range ready {
			if !running[phase] {
				toRun = append(toRun, phase)
			}
		}
		if len(toRun) == 0 {
			// All ready phases are somehow already running — shouldn't happen,
			// but treat like no runnable phases.
			fmt.Println("\nNo new phases ready to execute.")
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		// Launch all ready phases concurrently
		type phaseResult struct {
			phase string
			err   error
		}
		results := make(chan phaseResult, len(toRun))
		var wg sync.WaitGroup

		// When StopOnFailure is set, use a child context so we can cancel
		// sibling phases in the same batch on first failure.
		batchCtx, batchCancel := context.WithCancel(ctx)

		for _, phase := range toRun {
			running[phase] = true
			wg.Add(1)
			go func(phaseName string) {
				defer wg.Done()
				opts.Logger.Info("starting phase", "phase", phaseName)
				fmt.Printf("\n[%s] Starting phase\n", phaseName)

				err := RunPhase(batchCtx, RunPhaseOptions{
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
				})
				results <- phaseResult{phase: phaseName, err: err}
			}(phase)
		}

		// Close results channel after all goroutines complete.
		go func() {
			wg.Wait()
			close(results)
		}()

		// Process results as they arrive.
		var fatalErr error
		var failedPhase string
		for r := range results {
			delete(running, r.phase)
			if r.err != nil {
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
				if ctx.Err() != nil {
					batchCancel()
					return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
				}
				// Record the first fatal error but continue draining results
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
			return buildResult("failed", failedPhase, fatalErr.Error()), fatalErr
		}
	}
}

func loadAllPhaseStates(planDir string, phases []string) map[string]*arc.PhaseState {
	states := make(map[string]*arc.PhaseState, len(phases))
	for _, phase := range phases {
		states[phase] = loadPhaseState(planDir, phase)
	}
	return states
}

func loadPhaseState(planDir string, phase string) *arc.PhaseState {
	path := filepath.Join(planDir, "phases", phase, "state.json")
	sf := state.NewStateFile(path)
	ps, err := sf.Read()
	if err != nil {
		return nil
	}
	return ps
}

func printBlockedSummary(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) {
	for _, phase := range meta.Phases {
		ps := phaseStates[phase]
		if ps == nil {
			continue
		}
		if ps.PhaseStatus == "complete" || ps.PhaseStatus == "split" || ps.PhaseStatus == "deferred" {
			continue
		}
		blockers := []string{}
		for _, dep := range meta.Dependencies[phase] {
			depState := phaseStates[dep]
			if depState == nil || depState.PhaseStatus != "complete" {
				blockers = append(blockers, dep)
			}
		}
		if len(blockers) > 0 {
			fmt.Printf("  [%s] blocked by: %s\n", phase, strings.Join(blockers, ", "))
		} else if ps.PhaseStatus == "blocked" {
			fmt.Printf("  [%s] permanently blocked (max rollbacks)\n", phase)
		}
	}
}

// acquireLock creates <planDir>/.orchestrator.lock with current PID.
func acquireLock(planDir string) error {
	lockPath := filepath.Join(planDir, ".orchestrator.lock")

	data, err := os.ReadFile(lockPath)
	if err == nil {
		// Lock file exists - check if PID is still alive
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			// Check if process is alive by sending signal 0
			process, err := os.FindProcess(pid)
			if err == nil {
				err = process.Signal(syscall.Signal(0))
				if err == nil {
					return fmt.Errorf("orchestrator already running (PID %d)", pid)
				}
			}
		}
		// Stale lock - remove it
		os.Remove(lockPath)
	}

	// Write our PID
	return os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// releaseLock removes <planDir>/.orchestrator.lock.
func releaseLock(planDir string) {
	lockPath := filepath.Join(planDir, ".orchestrator.lock")
	os.Remove(lockPath)
}

// printUsageSummary outputs a JSON usage summary to stdout.
func printUsageSummary(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) {
	phases := make(map[string]*arc.Usage, len(meta.Phases))
	var total arc.Usage
	for _, phase := range meta.Phases {
		ps := phaseStates[phase]
		if ps == nil || ps.Usage.IsZero() {
			continue
		}
		u := ps.Usage
		phases[phase] = &u
		total = total.Add(u)
	}
	if total.IsZero() {
		return
	}
	summary := struct {
		Phases map[string]*arc.Usage `json:"phases"`
		Total  arc.Usage             `json:"total"`
	}{
		Phases: phases,
		Total:  total,
	}
	data, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Printf("\n%s\n", data)
}
