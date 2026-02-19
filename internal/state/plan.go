package state

import "github.com/nwiley/arc/internal/arc"

// ReadPlan reads plan.json from the given plan directory.
func ReadPlan(planDir string) (*arc.PlanMeta, error) {
	panic("not implemented")
}

// WritePlan atomically writes plan.json to the given plan directory.
func WritePlan(planDir string, meta *arc.PlanMeta) error {
	panic("not implemented")
}

// NextPhase returns the next incomplete phase in dependency order, or "" if all complete.
func NextPhase(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) string {
	panic("not implemented")
}

// PhasesReady returns all phases whose dependencies are all complete.
func PhasesReady(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) []string {
	panic("not implemented")
}
