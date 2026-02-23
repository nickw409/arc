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
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
)

// LaunchOptions configures the orchestrator launcher.
type LaunchOptions struct {
	PlanName   string
	PlansDir   string
	ArcHome    string
	ProjectDir string // working directory for git commits; empty uses process cwd
	Config     *config.Config
	Logger     *slog.Logger
	Timeout    int // wall-clock timeout in seconds (0 = no timeout)
}

// Launch starts the orchestrator for a plan.
func Launch(ctx context.Context, opts LaunchOptions) error {
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	if err := acquireLock(planDir); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
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
		return fmt.Errorf("reading plan.json: %w", err)
	}

	// Clean up review output files from previous adversarial reviews
	if err := review.CleanupOutputFiles(planDir, meta.Phases); err != nil {
		opts.Logger.Warn("failed to clean review output files", "error", err)
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

	// Main orchestration loop
	for {
		if ctx.Err() != nil {
			fmt.Println("\nOrchestrator timed out or cancelled.")
			fmt.Println("Re-run to continue from where it left off.")
			return ctx.Err()
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
			return generateCompletionReport(planDir, opts.PlanName, meta, phaseStates)
		}

		// Find next ready phase
		next := state.NextPhase(meta, phaseStates)
		if next == "" {
			// No phases are ready — all remaining are blocked by dependencies
			fmt.Println("\nNo phases ready to execute. Remaining phases are blocked by dependencies.")
			printBlockedSummary(meta, phaseStates)
			return fmt.Errorf("no runnable phases")
		}

		opts.Logger.Info("starting phase", "phase", next)
		fmt.Printf("\n[%s] Starting phase\n", next)

		err := RunPhase(ctx, RunPhaseOptions{
			PlanName:   opts.PlanName,
			PhaseName:  next,
			PlansDir:   opts.PlansDir,
			ArcHome:    opts.ArcHome,
			ProjectDir: opts.ProjectDir,
			Config:     opts.Config,
			Logger:     opts.Logger,
		})

		if err != nil {
			// Check if it's a blocked error — continue to next phase
			ps := loadPhaseState(planDir, next)
			if ps != nil && (ps.PhaseStatus == "blocked" || ps.PhaseStatus == "deferred") {
				fmt.Printf("[%s] Phase %s, continuing...\n", next, ps.PhaseStatus)
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("phase %s failed: %w", next, err)
		}

		fmt.Printf("[%s] Complete\n", next)
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

// ensure imports used
var _ = json.Unmarshal
