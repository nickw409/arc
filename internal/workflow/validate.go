package workflow

import (
	"fmt"

	"github.com/nwiley/arc/internal/arc"
)

// ValidationError represents a specific validation failure.
type ValidationError struct {
	Field   string
	Message string
}

// Error formats as "field: message".
func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// Validate checks a workflow for structural errors.
func Validate(w *arc.Workflow) []error {
	var errs []error

	// 8. empty states list
	if len(w.States) == 0 {
		errs = append(errs, &ValidationError{Field: "states", Message: "no states defined"})
		return errs
	}

	// Build state name set
	stateNames := make(map[string]bool, len(w.States))

	// 4. no duplicate state names
	for i, s := range w.States {
		if stateNames[s.Name] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("states[%d].name", i),
				Message: fmt.Sprintf("duplicate state name %q", s.Name),
			})
		}
		stateNames[s.Name] = true
	}

	// 1. entry_state exists in states
	if !stateNames[w.EntryState] {
		errs = append(errs, &ValidationError{
			Field:   "entry_state",
			Message: fmt.Sprintf("entry state %q not found in states", w.EntryState),
		})
	}

	// 3. all terminal_states exist in states
	terminalSet := make(map[string]bool, len(w.TerminalStates))
	for _, ts := range w.TerminalStates {
		terminalSet[ts] = true
		if !stateNames[ts] {
			errs = append(errs, &ValidationError{
				Field:   "terminal_states",
				Message: fmt.Sprintf("terminal state %q does not exist in states", ts),
			})
		}
	}

	// 2. entry_state is not in terminal_states
	if terminalSet[w.EntryState] && stateNames[w.EntryState] {
		errs = append(errs, &ValidationError{
			Field:   "entry_state",
			Message: fmt.Sprintf("entry state %q is terminal", w.EntryState),
		})
	}

	// 5. all "next" references point to existing states
	for i, s := range w.States {
		if s.Transition.Branches != nil {
			for k, target := range s.Transition.Branches {
				if !stateNames[target] {
					field := fmt.Sprintf("states[%d].next", i)
					if k != "" {
						field = fmt.Sprintf("states[%d].next[%s]", i, string(k))
					}
					errs = append(errs, &ValidationError{
						Field:   field,
						Message: fmt.Sprintf("references unknown state %q", target),
					})
				}
			}
		}

		// 7. verdicts/branches set equality
		if len(s.Verdicts) > 0 {
			verdictSet := make(map[arc.Verdict]bool, len(s.Verdicts))
			for _, v := range s.Verdicts {
				verdictSet[arc.Verdict(v)] = true
			}

			branchSet := make(map[arc.Verdict]bool)
			if s.Transition.Branches != nil {
				for k := range s.Transition.Branches {
					branchSet[k] = true
				}
			}

			// Check for missing branches (verdict without branch)
			for v := range verdictSet {
				if !branchSet[v] {
					errs = append(errs, &ValidationError{
						Field:   fmt.Sprintf("states[%d].next", i),
						Message: fmt.Sprintf("verdict/branch mismatch: verdict %q has no corresponding branch", v),
					})
				}
			}

			// Check for extra branches (branch key not in verdicts)
			for k := range branchSet {
				if !verdictSet[k] {
					errs = append(errs, &ValidationError{
						Field:   fmt.Sprintf("states[%d].next", i),
						Message: fmt.Sprintf("verdict/branch mismatch: branch key %q is not in verdicts list", k),
					})
				}
			}
		}
	}

	// 6. all states reachable from entry_state via BFS (forward reachability)
	if stateNames[w.EntryState] {
		reachable := make(map[string]bool)
		queue := []string{w.EntryState}
		reachable[w.EntryState] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			for _, s := range w.States {
				if s.Name == current && s.Transition.Branches != nil {
					for _, target := range s.Transition.Branches {
						if !reachable[target] {
							reachable[target] = true
							queue = append(queue, target)
						}
					}
				}
			}
		}

		for _, s := range w.States {
			if !reachable[s.Name] && !terminalSet[s.Name] {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("states.%s", s.Name),
					Message: fmt.Sprintf("state %q is unreachable from entry state %q", s.Name, w.EntryState),
				})
			}
		}
	}

	// 9. reverse reachability: all non-terminal states can reach a terminal state
	if len(terminalSet) > 0 && stateNames[w.EntryState] {
		// Build reverse edge map
		reverseEdges := make(map[string][]string)
		for _, s := range w.States {
			if s.Transition.Branches != nil {
				for _, target := range s.Transition.Branches {
					reverseEdges[target] = append(reverseEdges[target], s.Name)
				}
			}
		}

		// BFS backward from terminal states
		canReachTerminal := make(map[string]bool)
		queue := make([]string, 0, len(w.TerminalStates))
		for _, ts := range w.TerminalStates {
			canReachTerminal[ts] = true
			queue = append(queue, ts)
		}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, pred := range reverseEdges[current] {
				if !canReachTerminal[pred] {
					canReachTerminal[pred] = true
					queue = append(queue, pred)
				}
			}
		}

		for _, s := range w.States {
			if !terminalSet[s.Name] && !canReachTerminal[s.Name] {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("states.%s", s.Name),
					Message: fmt.Sprintf("state %q cannot reach any terminal state (possible infinite cycle)", s.Name),
				})
			}
		}
	}

	return errs
}
