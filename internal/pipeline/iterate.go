package pipeline

import (
	"context"
	"log/slog"

	"github.com/nwiley/arc/internal/arc"
)

// agentCommandName is the binary name used for agent spawning.
// Tests override this to point to a mock binary.
var agentCommandName = "claude"

// IterateOptions configures a single iteration.
type IterateOptions struct {
	PlanName     string
	PhaseName    string
	Mode         string
	Instructions string
	PlansDir     string
	ArcHome      string
}

// RunIteration executes a single iteration of the phase state machine.
func RunIteration(ctx context.Context, logger *slog.Logger, opts IterateOptions) *arc.IterationResult {
	panic("not implemented")
}

type escalationAction struct {
	Action string
	Params map[string]string
}

func checkIntervention(state *arc.PhaseState, triggers []arc.InterventionTrigger) string {
	return ""
}

func checkEscalation(state *arc.PhaseState, rules []arc.EscalationRule) *escalationAction {
	return nil
}

func checkPreConstraints(state *arc.PhaseState, constraints *arc.ConstraintConfig, phaseDir string) error {
	return nil
}

func checkPostConstraints(constraints *arc.ConstraintConfig, phaseDir string) error {
	return nil
}

func runAfterHooks(ctx context.Context, hooks []arc.HookConfig, verdict arc.Verdict, state *arc.PhaseState, phaseDir string) error {
	return nil
}

// MapStateToStatus maps a workflow state name to a phase_status value.
func MapStateToStatus(stateName string) string {
	panic("not implemented")
}
