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

	"github.com/nwiley/arc/internal/arc"
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
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	if err := acquireLock(planDir); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer releaseLock(planDir)

	// Load plan.json
	planData, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		return fmt.Errorf("reading plan.json: %w", err)
	}
	var planMeta arc.PlanMeta
	if err := json.Unmarshal(planData, &planMeta); err != nil {
		return fmt.Errorf("parsing plan.json: %w", err)
	}

	// Run phases in order
	for _, phase := range planMeta.Phases {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		opts.Logger.Info("starting phase", "phase", phase)
		if err := RunPhase(ctx, RunPhaseOptions{
			PlanName:  opts.PlanName,
			PhaseName: phase,
			PlansDir:  opts.PlansDir,
			ArcHome:   opts.ArcHome,
			Logger:    opts.Logger,
		}); err != nil {
			return fmt.Errorf("phase %s failed: %w", phase, err)
		}
	}

	return nil
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
