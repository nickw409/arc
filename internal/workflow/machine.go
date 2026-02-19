package workflow

import (
	"fmt"

	"github.com/nwiley/arc/internal/arc"
)

// Terminal is the sentinel value returned by NextState for terminal states.
const Terminal = "TERMINAL"

// Machine provides state navigation for a loaded workflow.
type Machine struct {
	workflow *arc.Workflow
	states   map[string]*arc.StateConfig
}

// NewMachine creates a Machine from a loaded workflow.
func NewMachine(w *arc.Workflow) *Machine {
	m := &Machine{
		workflow: w,
		states:   make(map[string]*arc.StateConfig),
	}
	if w != nil {
		for i := range w.States {
			m.states[w.States[i].Name] = &w.States[i]
		}
	}
	return m
}

// NextState resolves the next state given current state and verdict.
func (m *Machine) NextState(current string, verdict arc.Verdict) (string, error) {
	sc := m.states[current]
	if sc == nil {
		return "", fmt.Errorf("state %q not found in workflow", current)
	}

	if m.IsTerminal(current) {
		return Terminal, nil
	}

	// Linear state: no verdicts defined
	if len(sc.Verdicts) == 0 {
		if sc.Transition.Branches != nil {
			return sc.Transition.Branches[""], nil
		}
		return "", fmt.Errorf("state %q has no transition defined", current)
	}

	// Branching state: requires verdict
	if verdict == "" {
		return "", fmt.Errorf("state %q requires a verdict for transition", current)
	}

	next, ok := sc.Transition.Branches[verdict]
	if !ok {
		return "", fmt.Errorf("verdict %q not valid for state %q", verdict, current)
	}
	return next, nil
}

// IsTerminal returns true if the state name is in the workflow's terminal_states list.
func (m *Machine) IsTerminal(state string) bool {
	if m.workflow == nil {
		return false
	}
	for _, ts := range m.workflow.TerminalStates {
		if ts == state {
			return true
		}
	}
	return false
}

// EntryState returns the workflow's entry_state name.
func (m *Machine) EntryState() string {
	if m.workflow == nil {
		return ""
	}
	return m.workflow.EntryState
}

// GetState returns the StateConfig for the given state name, or nil if not found.
func (m *Machine) GetState(name string) *arc.StateConfig {
	return m.states[name]
}

// ValidVerdicts returns the valid verdicts for a state as []arc.Verdict,
// or nil if the state has no verdicts (linear state).
func (m *Machine) ValidVerdicts(state string) []arc.Verdict {
	sc := m.states[state]
	if sc == nil || len(sc.Verdicts) == 0 {
		return nil
	}
	verdicts := make([]arc.Verdict, len(sc.Verdicts))
	for i, v := range sc.Verdicts {
		verdicts[i] = arc.Verdict(v)
	}
	return verdicts
}
