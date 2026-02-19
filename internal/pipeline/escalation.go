package pipeline

import "github.com/nwiley/arc/internal/arc"

// EscalationAction represents a triggered escalation.
type EscalationAction struct {
	Rule   arc.EscalationRule
	Action string
	Params map[string]string
}

// CheckEscalation evaluates escalation rules against the current state.
func CheckEscalation(state *arc.PhaseState, rules []arc.EscalationRule) *EscalationAction {
	panic("not implemented")
}
