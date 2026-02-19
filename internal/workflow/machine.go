package workflow

import "github.com/nwiley/arc/internal/arc"

// Terminal is the sentinel value returned by NextState for terminal states.
const Terminal = "TERMINAL"

// Machine provides state navigation for a loaded workflow.
type Machine struct {
	workflow *arc.Workflow
	states   map[string]*arc.StateConfig
}

// NewMachine creates a Machine from a loaded workflow.
func NewMachine(w *arc.Workflow) *Machine {
	panic("not implemented")
}

// NextState resolves the next state given current state and verdict.
func (m *Machine) NextState(current string, verdict arc.Verdict) (string, error) {
	panic("not implemented")
}

// IsTerminal returns true if the state name is in the workflow's terminal_states list.
func (m *Machine) IsTerminal(state string) bool {
	panic("not implemented")
}

// EntryState returns the workflow's entry_state name.
func (m *Machine) EntryState() string {
	panic("not implemented")
}

// GetState returns the StateConfig for the given state name, or nil if not found.
func (m *Machine) GetState(name string) *arc.StateConfig {
	panic("not implemented")
}

// ValidVerdicts returns the valid verdicts for a state as []arc.Verdict,
// or nil if the state has no verdicts (linear state).
func (m *Machine) ValidVerdicts(state string) []arc.Verdict {
	panic("not implemented")
}
