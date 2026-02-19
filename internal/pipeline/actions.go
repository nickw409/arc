package pipeline

import (
	"context"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
)

// ActionContext provides dependencies needed by actions.
type ActionContext struct {
	PhaseDir string
	PlanName string
	Phase    string
	Config   *config.Config
	State    *arc.PhaseState
	ArcHome  string
}

// RunAction executes a named action with the given parameters.
func RunAction(ctx context.Context, action string, params map[string]string, actx ActionContext) error {
	panic("not implemented")
}
