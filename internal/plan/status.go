package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// StatusOptions configures status display.
type StatusOptions struct {
	PlansDir string
	PlanName string
}

// Status writes plan status to the given writer.
func Status(w io.Writer, opts StatusOptions) error {
	if opts.PlanName != "" {
		return statusForPlan(w, opts.PlansDir, opts.PlanName)
	}

	// Show all plans
	entries, err := os.ReadDir(opts.PlansDir)
	if err != nil {
		return fmt.Errorf("read plans directory: %w", err)
	}

	var planNames []string
	for _, e := range entries {
		if e.IsDir() {
			planNames = append(planNames, e.Name())
		}
	}
	sort.Strings(planNames)

	for _, name := range planNames {
		if err := statusForPlan(w, opts.PlansDir, name); err != nil {
			return err
		}
	}
	return nil
}

func statusForPlan(w io.Writer, plansDir, planName string) error {
	planDir := filepath.Join(plansDir, planName)

	// Read plan.json
	planData, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		return fmt.Errorf("read plan.json for %s: %w", planName, err)
	}

	var meta arc.PlanMeta
	if err := json.Unmarshal(planData, &meta); err != nil {
		return fmt.Errorf("parse plan.json for %s: %w", planName, err)
	}

	fmt.Fprintf(w, "Plan: %s (%s)\n", meta.Name, meta.WorkflowType)

	// Collect all phase states for dependency checking
	phaseStates := make(map[string]*arc.PhaseState)
	for _, phase := range meta.Phases {
		statePath := filepath.Join(planDir, "phases", phase, "state.json")
		stateData, err := os.ReadFile(statePath)
		if err != nil {
			phaseStates[phase] = nil
			continue
		}
		var state arc.PhaseState
		if err := json.Unmarshal(stateData, &state); err != nil {
			phaseStates[phase] = nil
			continue
		}
		phaseStates[phase] = &state
	}

	for _, phase := range meta.Phases {
		state := phaseStates[phase]
		if state == nil {
			fmt.Fprintf(w, "  [?] %s — error reading state\n", phase)
			continue
		}

		icon := StatusIcon(state.PhaseStatus)
		line := fmt.Sprintf("  %s %s", icon, phase)

		// Add adversary round if applicable
		if state.AdversaryRound > 0 {
			line += fmt.Sprintf(" adversary-round:%d", state.AdversaryRound)
		}

		// Add iteration info if in progress
		if iter := state.StateIterations[state.CurrentState]; iter > 0 {
			line += fmt.Sprintf(" iter %d", iter)
		}

		// Add test counts if present
		if state.TestsTotal > 0 {
			line += fmt.Sprintf(" %d/%d", state.TestsPassing, state.TestsTotal)
		}

		// Add dispute count if present
		if len(state.Disputes) > 0 {
			line += fmt.Sprintf(" disputes:%d", len(state.Disputes))
		}

		// Add activity if present
		if state.Activity != "" {
			activity := state.Activity
			if len(activity) > 60 {
				activity = activity[:57] + "..."
			}
			line += fmt.Sprintf(" — %s", activity)
		}

		// Check blocked-by deps
		deps := meta.Dependencies[phase]
		var blockedBy []string
		for _, dep := range deps {
			depState := phaseStates[dep]
			if depState == nil || depState.PhaseStatus != "complete" {
				blockedBy = append(blockedBy, dep)
			}
		}
		if len(blockedBy) > 0 {
			line += fmt.Sprintf(" BLOCKED BY %s", strings.Join(blockedBy, ", "))
		}

		fmt.Fprintln(w, line)
	}

	return nil
}

// StatusIcon returns the display icon for a phase status string.
func StatusIcon(status string) string {
	switch status {
	case "pending":
		return "[ ]"
	case "complete":
		return "[x]"
	case "blocked":
		return "[X]"
	case "adversary":
		return "[!]"
	case "disputed":
		return "[!]"
	case "deferred":
		return "[~]"
	case "split":
		return "[/]"
	default:
		return "[>]"
	}
}
