package review

import (
	"context"
	"log/slog"
)

// ReviewOptions configures the adversarial review loop.
type ReviewOptions struct {
	PlanName string
	PlansDir string
	ArcHome  string
	Phase    string
	Logger   *slog.Logger
}

// ReviewResult holds the outcome of a review.
type ReviewResult struct {
	Status    string
	Verdicts  map[string]AdversaryResult
	Iteration int
}

// AdversaryResult holds the outcome of a single adversary agent.
type AdversaryResult struct {
	Name     string
	Verdict  string
	Status   string
	Output   string
	Required bool
}

// Run executes the adversarial review loop.
func Run(ctx context.Context, opts ReviewOptions) (*ReviewResult, error) {
	panic("not implemented")
}
