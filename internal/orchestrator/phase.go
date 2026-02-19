package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/pipeline"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/workflow"
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
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// Load state
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// Load workflow
	wfBytes, err := resources.WorkflowBytes(phaseState.WorkflowType)
	if err != nil {
		return fmt.Errorf("loading workflow: %w", err)
	}
	wf, err := workflow.LoadBytes(wfBytes)
	if err != nil {
		return fmt.Errorf("parsing workflow: %w", err)
	}
	machine := workflow.NewMachine(wf)

	// Set initial state if empty
	if phaseState.CurrentState == "" {
		phaseState.CurrentState = machine.EntryState()
		if err := sf.Write(phaseState); err != nil {
			return fmt.Errorf("writing initial state: %w", err)
		}
	}

	// Run iteration loop
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Re-read state to pick up any changes
		phaseState, err = sf.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		// Check if terminal
		if machine.IsTerminal(phaseState.CurrentState) {
			return nil
		}

		// Derive mode from state name
		mode := deriveMode(phaseState.CurrentState)

		// Run iteration
		result := pipeline.RunIteration(ctx, opts.Logger, pipeline.IterateOptions{
			PlanName:  opts.PlanName,
			PhaseName: opts.PhaseName,
			Mode:      mode,
			PlansDir:  opts.PlansDir,
			ArcHome:   opts.ArcHome,
		})

		if result.Err != nil {
			switch result.Action {
			case arc.ActionAbort:
				return result.Err
			case arc.ActionRetry:
				opts.Logger.Warn("iteration failed, retrying", "error", result.Err)
				continue
			default:
				return result.Err
			}
		}

		if result.Action == arc.ActionContinue && machine.IsTerminal(result.NextState) {
			return nil
		}
	}
}

// deriveMode maps a workflow state name to a prompt mode string.
func deriveMode(stateName string) string {
	var mode string
	if strings.Contains(stateName, "review") {
		mode = stateName
	} else if stateName == "qa" {
		mode = "qa"
	} else {
		mode = "impl"
	}
	return strings.ReplaceAll(mode, "_", "-")
}

// ensure json import is used (for plan.json loading in Launch)
var _ = json.Unmarshal
var _ = os.ReadFile
