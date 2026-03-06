package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
)

// LaunchOptions configures the orchestrator launcher.
type LaunchOptions struct {
	PlanName         string
	PlansDir         string
	ArcHome          string
	ProjectDir       string              // working directory for git commits; empty uses process cwd
	Config           *config.Config
	ConfigPath       string              // path to .arc.yaml; used by SIGHUP handler to reload config
	Logger           *slog.Logger
	Timeout          int                 // wall-clock timeout in seconds (0 = no timeout)
	UseWorktree      bool                // if true, run agents in isolated git worktrees
	PerPhaseWorktree bool                // if true, create a separate worktree per phase instead of one shared worktree
	StopOnFailure    bool                // if true, cancel in-progress phases and return on first failure
	ChatMode         bool                // if true, skip escalation ladder and block immediately for chat-agent intervention
	Resolver         *resources.Resolver // passed through to RunPhaseGatedOptions
	PlanLogger       *PlanLogger         // if non-nil, reuse this logger instead of creating a new one
}

// LaunchResult describes the outcome of an orchestrator run.
type LaunchResult struct {
	Status       string            // "complete", "failed", "cancelled", "blocked"
	FailedPhase  string            // which phase caused the stop (empty if complete)
	FailedReason string            // why it failed
	PhaseSummary map[string]string // phase name → final status
	Usage        arc.Usage
}

// Launch starts the gate-based orchestrator for a plan.
func Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	return LaunchGated(ctx, opts)
}

// shortHash returns the first 7 characters of a hash, or the full string if shorter.
func shortHash(hash string) string {
	if len(hash) >= 7 {
		return hash[:7]
	}
	return hash
}

// LoadAllPhaseStates reads state.json for each phase and returns a map.
func LoadAllPhaseStates(planDir string, phases []string) map[string]*arc.PhaseState {
	states := make(map[string]*arc.PhaseState, len(phases))
	for _, phase := range phases {
		states[phase] = LoadPhaseState(planDir, phase)
	}
	return states
}

// LoadPhaseState reads a single phase's state.json. Returns nil if not found.
func LoadPhaseState(planDir string, phase string) *arc.PhaseState {
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

// AcquirePlanLock creates <planDir>/.orchestrator.lock with the given PID.
// If a lock already exists for a live process, it returns an error.
func AcquirePlanLock(planDir string, pid int) error {
	lockPath := filepath.Join(planDir, ".orchestrator.lock")

	data, err := os.ReadFile(lockPath)
	if err == nil {
		// Lock file exists - check if PID is still alive
		existingPid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			process, err := os.FindProcess(existingPid)
			if err == nil {
				err = process.Signal(syscall.Signal(0))
				if err == nil {
					return fmt.Errorf("orchestrator already running (PID %d)", existingPid)
				}
			}
		}
		// Stale lock - remove it
		os.Remove(lockPath)
	}

	return os.WriteFile(lockPath, []byte(strconv.Itoa(pid)), 0644)
}

// ReleasePlanLock removes <planDir>/.orchestrator.lock.
func ReleasePlanLock(planDir string) {
	lockPath := filepath.Join(planDir, ".orchestrator.lock")
	os.Remove(lockPath)
}

// CheckPlanLock reads the lock file and returns the PID and whether the lock is held
// by a live process.
func CheckPlanLock(planDir string) (pid int, locked bool) {
	lockPath := filepath.Join(planDir, ".orchestrator.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}

// acquireLock creates <planDir>/.orchestrator.lock with current PID.
func acquireLock(planDir string) error {
	return AcquirePlanLock(planDir, os.Getpid())
}

// releaseLock removes <planDir>/.orchestrator.lock.
func releaseLock(planDir string) {
	ReleasePlanLock(planDir)
}

