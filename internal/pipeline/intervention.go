package pipeline

import (
	"strconv"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// CheckIntervention evaluates intervention triggers against the current state.
func CheckIntervention(state *arc.PhaseState, triggers []arc.InterventionTrigger) string {
	if state == nil {
		return ""
	}
	for _, trigger := range triggers {
		tokens := strings.Fields(trigger.Condition)
		if len(tokens) != 3 {
			continue
		}
		field, op, valStr := tokens[0], tokens[1], tokens[2]

		fieldVal, ok := getFieldValue(state, field)
		if !ok {
			continue
		}

		val, err := strconv.Atoi(valStr)
		if err != nil {
			continue
		}

		if evalCondition(fieldVal, op, val) {
			return trigger.Message
		}
	}
	return ""
}

func getFieldValue(state *arc.PhaseState, field string) (int, bool) {
	switch field {
	case "stuck_iterations":
		return state.StuckIterations, true
	case "hang_count":
		return state.HangCount, true
	case "iteration":
		return state.Iteration.Current, true
	case "tests_passing":
		return state.TestsPassing, true
	case "tests_total":
		return state.TestsTotal, true
	case "rollback_count":
		return state.RollbackCount, true
	case "global_iterations":
		return state.GlobalIterations, true
	default:
		return 0, false
	}
}

func evalCondition(fieldVal int, op string, val int) bool {
	switch op {
	case "==":
		return fieldVal == val
	case "!=":
		return fieldVal != val
	case ">":
		return fieldVal > val
	case "<":
		return fieldVal < val
	case ">=":
		return fieldVal >= val
	case "<=":
		return fieldVal <= val
	default:
		return false
	}
}
