package review

import (
	"context"
	"log/slog"
	"path/filepath"
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	adversaries := DefaultAdversaries()

	result := &ReviewResult{
		Verdicts: make(map[string]AdversaryResult),
	}

	for _, adv := range adversaries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		advResult, err := RunAdversary(ctx, adv, planDir, opts.Phase)
		if err != nil {
			result.Verdicts[adv.Name] = AdversaryResult{
				Name:     adv.Name,
				Status:   "error",
				Output:   err.Error(),
				Required: adv.Required,
			}
			continue
		}
		result.Verdicts[adv.Name] = *advResult
	}

	// Determine overall status
	result.Status = determineReviewStatus(result.Verdicts)

	opts.Logger.Info("review complete", "status", result.Status, "phase", opts.Phase)
	return result, nil
}

func determineReviewStatus(verdicts map[string]AdversaryResult) string {
	allPassed := true
	requiredFailed := false

	for _, v := range verdicts {
		if v.Status != "passed" && v.Status != "cached" {
			allPassed = false
			if v.Required {
				requiredFailed = true
			}
		}
	}

	if allPassed {
		return "approved"
	}
	if requiredFailed {
		return "needs_review"
	}
	return "conditional"
}
