package review

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxReviewIterations is the maximum number of review iterations per phase.
const MaxReviewIterations = 5

// ReviewOptions configures the adversarial review loop.
type ReviewOptions struct {
	PlanName string
	PlansDir string
	ArcHome  string
	Phase    string
	Model    string
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
	Name         string
	Verdict      string
	Status       string
	CachedStatus string // original status before caching (only set when Status=="cached")
	Output       string
	Required     bool
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

	// Load history and check iteration limit
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	history := LoadHistory(histPath)

	if history.Iterations[opts.Phase] >= MaxReviewIterations {
		return nil, fmt.Errorf("phase %q has reached the maximum of %d review iterations; use 'arc manage reset-review' to reset", opts.Phase, MaxReviewIterations)
	}

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

			advResult, err := RunAdversary(ctx, a, planDir, opts.Phase, planMD, opts.Model)
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

	// Write adversary output files for non-cached, non-error results
	reviewsDir := filepath.Join(planDir, "reviews")
	os.MkdirAll(reviewsDir, 0755)
	for _, v := range result.Verdicts {
		if v.Status == "cached" || v.Status == "error" {
			continue
		}
		outPath := filepath.Join(reviewsDir, opts.Phase+"_"+v.Name+".md")
		os.WriteFile(outPath, []byte(v.Output), 0644)
	}

	// Save history after all goroutines complete (no concurrent writes)
	hash, _ := computePlanHash(planMDPath)
	if history.Phases[opts.Phase] == nil {
		history.Phases[opts.Phase] = make(map[string]historyEntry)
	}
	hasNonCached := false
	for _, v := range result.Verdicts {
		if v.Status == "cached" || v.Status == "error" {
			continue
		}
		hasNonCached = true
		history.Phases[opts.Phase][v.Name] = historyEntry{
			Hash:      hash,
			Verdict:   v.Verdict,
			Status:    v.Status,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}
	if hasNonCached {
		history.Iterations[opts.Phase]++
	}
	result.Iteration = history.Iterations[opts.Phase]
	SaveHistory(histPath, history)

	// Determine overall status
	result.Status = determineReviewStatus(result.Verdicts)

	opts.Logger.Info("review complete", "status", result.Status, "phase", opts.Phase)
	return result, nil
}

// CleanupOutputFiles removes adversary output files for the given phases.
// It preserves adversary_history.json and any other non-output files.
func CleanupOutputFiles(planDir string, phases []string) error {
	reviewsDir := filepath.Join(planDir, "reviews")
	adversaries := DefaultAdversaries()
	for _, phase := range phases {
		for _, adv := range adversaries {
			path := filepath.Join(reviewsDir, phase+"_"+adv.Name+".md")
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s: %w", path, err)
			}
		}
	}
	return nil
}

func determineReviewStatus(verdicts map[string]AdversaryResult) string {
	allPassed := true
	requiredFailed := false

	for _, v := range verdicts {
		isPassing := v.Status == "passed" || (v.Status == "cached" && v.CachedStatus == "passed")
		if !isPassing {
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
