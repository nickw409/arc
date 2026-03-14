package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/gate"
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

	// Show orchestrator status if a PID file exists.
	pidPath := filepath.Join(planDir, "orchestrator.pid")
	if pidData, err := os.ReadFile(pidPath); err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if parseErr == nil {
			if isProcessAlive(pid) {
				fmt.Fprintf(w, "  Orchestrator: running (PID %d)\n", pid)
			} else {
				fmt.Fprintf(w, "  Orchestrator: not running (stale PID %d)\n", pid)
			}
		}
	}

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
		if iter := state.Iteration.Current; iter > 0 {
			line += fmt.Sprintf(" iter %d", iter)
		}

		// Add test counts if present
		if state.TestsTotal > 0 {
			line += fmt.Sprintf(" %d/%d", state.TestsPassing, state.TestsTotal)
		}

		// For terminal phases, show status summary instead of stale activity.
		switch state.PhaseStatus {
		case "complete":
			line += " — complete"
			if state.CompletedAt != "" {
				line += fmt.Sprintf(" (%s)", state.CompletedAt)
			}
		case "blocked":
			reason := state.BlockedReason
			if reason == "" {
				reason = "unknown reason"
			}
			if len(reason) > 50 {
				reason = reason[:47] + "..."
			}
			line += fmt.Sprintf(" — blocked: %s", reason)
		case "deferred":
			reason := state.DeferredReason
			if reason == "" {
				reason = "no reason given"
			}
			if len(reason) > 50 {
				reason = reason[:47] + "..."
			}
			line += fmt.Sprintf(" — deferred: %s", reason)
		case "split":
			line += " — split"
		default:
			// Non-terminal: show live activity.
			if state.Activity != "" {
				activity := state.Activity
				if len(activity) > 60 {
					activity = activity[:57] + "..."
				}
				line += fmt.Sprintf(" — %s", activity)
			}
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

		// Show checkpoint results if gate-status.json exists.
		phaseDir := filepath.Join(planDir, "phases", phase)
		if gs, err := gate.ReadStatus(phaseDir); err == nil && len(gs.Checkpoints) > 0 {
			// Read spec to get checkpoint order; fall back to map iteration.
			var cpNames []string
			if spec, specErr := ReadSpec(plansDir, planName, phase); specErr == nil {
				for _, cp := range spec.Checkpoints {
					if _, ok := gs.Checkpoints[cp.Name]; ok {
						cpNames = append(cpNames, cp.Name)
					}
				}
			}
			// Include any checkpoint names in gate-status but not in spec.
			inSpec := make(map[string]bool, len(cpNames))
			for _, n := range cpNames {
				inSpec[n] = true
			}
			for n := range gs.Checkpoints {
				if !inSpec[n] {
					cpNames = append(cpNames, n)
				}
			}
			if len(cpNames) > 0 {
				var parts []string
				for _, n := range cpNames {
					status := gs.Checkpoints[n]
					switch status {
					case "pass":
						parts = append(parts, n+"=pass")
					case "fail":
						parts = append(parts, n+"=FAIL")
					default:
						parts = append(parts, n+"="+status)
					}
				}
				fmt.Fprintf(w, "      checkpoints: %s\n", strings.Join(parts, "  "))
			}
		}
	}

	return nil
}

// AllPhasesTerminal returns true when every phase in every matching plan has
// reached a terminal status (complete, blocked, or deferred). It is used by
// the --live flag to decide when to stop polling.
func AllPhasesTerminal(opts StatusOptions) bool {
	checkPlan := func(plansDir, planName string) bool {
		planDir := filepath.Join(plansDir, planName)
		planData, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
		if err != nil {
			return false
		}
		var meta arc.PlanMeta
		if err := json.Unmarshal(planData, &meta); err != nil {
			return false
		}
		for _, phase := range meta.Phases {
			statePath := filepath.Join(planDir, "phases", phase, "state.json")
			stateData, err := os.ReadFile(statePath)
			if err != nil {
				return false
			}
			var st arc.PhaseState
			if err := json.Unmarshal(stateData, &st); err != nil {
				return false
			}
			switch st.PhaseStatus {
			case "complete", "blocked", "deferred", "split":
				// terminal
			default:
				return false
			}
		}
		return true
	}

	if opts.PlanName != "" {
		return checkPlan(opts.PlansDir, opts.PlanName)
	}

	entries, err := os.ReadDir(opts.PlansDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !checkPlan(opts.PlansDir, e.Name()) {
			return false
		}
	}
	return true
}

// WaitSummary returns a one-line summary of the plan's final state and whether
// any phase is blocked. Used by `arc wait` to report results.
func WaitSummary(plansDir, planName string) (summary string, hasBlocked bool) {
	planDir := filepath.Join(plansDir, planName)
	planData, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		return fmt.Sprintf("plan %q: cannot read plan.json", planName), false
	}
	var meta arc.PlanMeta
	if err := json.Unmarshal(planData, &meta); err != nil {
		return fmt.Sprintf("plan %q: cannot parse plan.json", planName), false
	}

	var complete, blocked, deferred, other int
	for _, phase := range meta.Phases {
		statePath := filepath.Join(planDir, "phases", phase, "state.json")
		stateData, readErr := os.ReadFile(statePath)
		if readErr != nil {
			other++
			continue
		}
		var st arc.PhaseState
		if jsonErr := json.Unmarshal(stateData, &st); jsonErr != nil {
			other++
			continue
		}
		switch st.PhaseStatus {
		case "complete", "split":
			complete++
		case "blocked":
			blocked++
			hasBlocked = true
		case "deferred":
			deferred++
		default:
			other++
		}
	}

	total := len(meta.Phases)
	if hasBlocked {
		summary = fmt.Sprintf("plan %q: %d/%d complete, %d blocked", planName, complete, total, blocked)
	} else {
		summary = fmt.Sprintf("plan %q: %d/%d complete", planName, complete, total)
	}
	if deferred > 0 {
		summary += fmt.Sprintf(", %d deferred", deferred)
	}
	return summary, hasBlocked
}

// isProcessAlive returns true if a process with the given PID is alive.
// It uses signal 0, which checks process existence without sending a real signal.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
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
