package pipeline

import "github.com/nwiley/arc/internal/arc"

// CheckPreConstraints validates constraints before a state executes.
func CheckPreConstraints(state *arc.PhaseState, constraints *arc.ConstraintConfig, phaseDir string) error {
	panic("not implemented")
}

// CheckPostConstraints validates constraints after a state executes.
func CheckPostConstraints(constraints *arc.ConstraintConfig, phaseDir string) error {
	panic("not implemented")
}
