package pipeline

import (
	"fmt"

	"github.com/nwiley/arc/internal/arc"
)

// EscalationAction represents a triggered escalation.
type EscalationAction struct {
	Rule   arc.EscalationRule
	Action string
	Params map[string]string
}

// CheckEscalation evaluates escalation rules against the current state.
func CheckEscalation(state *arc.PhaseState, rules []arc.EscalationRule) *EscalationAction {
	if state == nil {
		return nil
	}
	for _, rule := range rules {
		if ruleMatches(state, rule) {
			return &EscalationAction{
				Rule:   rule,
				Action: rule.Action,
				Params: rule.Params,
			}
		}
	}
	return nil
}

func ruleMatches(state *arc.PhaseState, rule arc.EscalationRule) bool {
	cur := state.Iteration.Current

	// at_iteration: triggers when Current == N exactly
	if rule.AtIteration != nil {
		n := *rule.AtIteration
		if n >= 0 && cur == n {
			return true
		}
	}

	// after_iteration: triggers when Current > N and not already executed
	if rule.AfterIteration != nil {
		n := *rule.AfterIteration
		if cur > n {
			key := fmt.Sprintf("after_iteration_%d", n)
			if !containsStr(state.ExecutedEscalations, key) {
				return true
			}
		}
	}

	// every_n_iterations: triggers when Current > 0 and Current % N == 0
	if rule.EveryNIterations != nil {
		n := *rule.EveryNIterations
		if n != 0 && cur > 0 && cur%n == 0 {
			return true
		}
	}

	return false
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
