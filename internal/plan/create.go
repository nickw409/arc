package plan

import "github.com/nwiley/arc/internal/arc"

// CreateOptions configures plan creation.
type CreateOptions struct {
	PlansDir     string
	Name         string
	Phases       []string
	WorkflowType string
}

// Create creates a new plan with directory structure, state files, and templates.
func Create(opts CreateOptions) (*arc.PlanMeta, error) {
	panic("not implemented")
}
