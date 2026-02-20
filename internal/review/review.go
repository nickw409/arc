package review

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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

	// Read plan.md content for the phase
	planMDPath := filepath.Join(planDir, "phases", opts.Phase, "plan.md")
	planMDBytes, err := os.ReadFile(planMDPath)
	if err != nil {
		return nil, fmt.Errorf("reading plan.md for phase %s: %w", opts.Phase, err)
	}
	planMD := string(planMDBytes)

	result := &ReviewResult{
		Verdicts: make(map[string]AdversaryResult),
	}

	var wg sync.WaitGroup
	resultsCh := make(chan AdversaryResult, len(adversaries))

	for _, adv := range adversaries {
		wg.Add(1)
		go func(a Adversary) {
			defer wg.Done()

			if ctx.Err() != nil {
				resultsCh <- AdversaryResult{
					Name:     a.Name,
					Status:   "error",
					Output:   ctx.Err().Error(),
					Required: a.Required,
				}
				return
			}

			advResult, err := RunAdversary(ctx, a, planDir, opts.Phase, planMD)
			if err != nil {
				resultsCh <- AdversaryResult{
					Name:     a.Name,
					Status:   "error",
					Output:   err.Error(),
					Required: a.Required,
				}
				return
			}
			resultsCh <- *advResult
		}(adv)
	}

	wg.Wait()
	close(resultsCh)

	for r := range resultsCh {
		result.Verdicts[r.Name] = r
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
