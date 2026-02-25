package review

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// MaxReviewIterations is the maximum number of review iterations per phase.
const MaxReviewIterations = 5

// ReviewOptions configures the adversarial review loop.
type ReviewOptions struct {
	PlanName      string
	PlansDir      string
	ArcHome       string
	Phase         string
	Model         string
	Logger        *slog.Logger
	MaxIterations int // if > 0, overrides MaxReviewIterations (default 5)
}

// ReviewResult holds the outcome of a review.
type ReviewResult struct {
	Status             string
	Verdicts           map[string]AdversaryResult
	Iteration          int
	SuggestionsApplied int
	IterationDetails   []IterationDetail
	Usage              arc.Usage
}

// IterationDetail records what happened in a single iteration of the review loop.
type IterationDetail struct {
	Iteration          int
	Status             string
	SuggestionsFound   int
	SuggestionsApplied int
	Verdicts           map[string]string // adversary name -> status
}

// AdversaryResult holds the outcome of a single adversary agent.
type AdversaryResult struct {
	Name         string
	Verdict      string
	Status       string
	CachedStatus string // original status before caching (only set when Status=="cached")
	Output       string
	Required     bool
	Usage        arc.Usage
}

// Run executes the adversarial review loop with auto-remediation.
// Each iteration: run all adversaries → parse suggestions from failures →
// merge by priority → apply to plan.md → repeat until converged or limit hit.
func Run(ctx context.Context, opts ReviewOptions) (*ReviewResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	adversaries := DefaultAdversaries()
	planMDPath := filepath.Join(planDir, "phases", opts.Phase, "plan.md")
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	reviewsDir := filepath.Join(planDir, "reviews")
	os.MkdirAll(reviewsDir, 0755)

	result := &ReviewResult{
		Verdicts: make(map[string]AdversaryResult),
	}

	// Track failure signatures for convergence detection.
	// If the same set of failing adversaries appears twice, we're oscillating.
	var failureSignatures []string

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Load plan.md for this iteration
		planMDBytes, err := os.ReadFile(planMDPath)
		if err != nil {
			return nil, fmt.Errorf("reading plan.md for phase %s: %w", opts.Phase, err)
		}
		planMD := string(planMDBytes)

		// Check iteration limit
		history := LoadHistory(histPath)
		maxIter := MaxReviewIterations
		if opts.MaxIterations > 0 {
			maxIter = opts.MaxIterations
		}
		if history.Iterations[opts.Phase] >= maxIter {
			opts.Logger.Info("max review iterations reached, proceeding as conditional",
				"phase", opts.Phase,
				"iterations", history.Iterations[opts.Phase],
			)
			if result.Status == "" || result.Status == "needs_review" {
				result.Status = "conditional"
			}
			result.Iteration = history.Iterations[opts.Phase]
			break
		}

		// Run all adversaries in parallel
		iteration := history.Iterations[opts.Phase] + 1
		verdicts := runAdversaries(ctx, adversaries, planDir, opts.Phase, planMD, opts.Model, iteration)

		// Aggregate usage from this iteration's adversaries
		for _, v := range verdicts {
			result.Usage = result.Usage.Add(v.Usage)
		}

		// Write output files and update history
		hash, _ := computePlanHash(planMDPath)
		if history.Phases[opts.Phase] == nil {
			history.Phases[opts.Phase] = make(map[string]historyEntry)
		}
		hasNonCached := false
		for _, v := range verdicts {
			if v.Status == "cached" || v.Status == "error" {
				continue
			}
			hasNonCached = true
			outPath := filepath.Join(reviewsDir, opts.Phase+"_"+v.Name+".md")
			os.WriteFile(outPath, []byte(v.Output), 0644)
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
		SaveHistory(histPath, history)

		status := determineReviewStatus(verdicts)
		currentIteration := history.Iterations[opts.Phase]

		// Update result with latest verdicts
		result.Verdicts = verdicts
		result.Iteration = currentIteration
		result.Status = status

		// Convergence detection: if we've seen this failure pattern before, stop
		converged := false
		if status != "approved" && status != "conditional" {
			sig := failureSignature(verdicts)
			if sig != "" {
				for _, prev := range failureSignatures {
					if sig == prev {
						opts.Logger.Info("convergence detected — same failure pattern repeating, stopping review",
							"phase", opts.Phase,
							"failing", sig,
						)
						converged = true
						break
					}
				}
				failureSignatures = append(failureSignatures, sig)
			}
		}

		// Collect suggestions from failed adversaries
		var allSuggestions []Suggestion
		for _, v := range verdicts {
			if v.Status == "failed" {
				suggestions := ParseSuggestions(v.Name, v.Output)
				suggestions = FilterByConfidence(suggestions, DefaultConfidenceThreshold)
				allSuggestions = append(allSuggestions, suggestions...)
			}
		}

		// Merge and apply suggestions
		merged := MergeSuggestions(allSuggestions)
		applied := 0
		if len(merged) > 0 {
			planMD, applied = ApplySuggestions(planMD, merged)
			if applied > 0 {
				os.WriteFile(planMDPath, []byte(planMD), 0644)
			}
		}

		result.SuggestionsApplied += applied

		// Record iteration detail
		verdictSummary := make(map[string]string, len(verdicts))
		for name, v := range verdicts {
			effectiveStatus := v.Status
			if v.Status == "cached" {
				effectiveStatus = v.CachedStatus
			}
			verdictSummary[name] = effectiveStatus
		}
		result.IterationDetails = append(result.IterationDetails, IterationDetail{
			Iteration:          currentIteration,
			Status:             status,
			SuggestionsFound:   len(allSuggestions),
			SuggestionsApplied: applied,
			Verdicts:           verdictSummary,
		})

		opts.Logger.Info("review iteration complete",
			"phase", opts.Phase,
			"iteration", currentIteration,
			"status", status,
			"suggestions_found", len(allSuggestions),
			"suggestions_applied", applied,
		)

		// Exit conditions
		if status == "approved" || status == "conditional" {
			break
		}
		if converged || applied == 0 {
			break
		}
		// Suggestions were applied, plan.md changed — loop for re-review
	}

	// When the review loop exits without full approval, allow the plan to
	// proceed as conditional rather than blocking. The auto-remediation has
	// done its best — remaining issues are logged but non-blocking.
	if result.Status == "needs_review" {
		result.Status = "conditional"
	}

	opts.Logger.Info("review complete", "status", result.Status, "phase", opts.Phase)
	return result, nil
}

// failureSignature returns a deterministic string identifying which adversaries
// are currently failing, used for convergence/oscillation detection.
func failureSignature(verdicts map[string]AdversaryResult) string {
	var names []string
	for _, v := range verdicts {
		effectiveStatus := v.Status
		if v.Status == "cached" {
			effectiveStatus = v.CachedStatus
		}
		if effectiveStatus == "failed" {
			names = append(names, v.Name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// runAdversaries spawns all adversaries concurrently and returns their results.
func runAdversaries(ctx context.Context, adversaries []Adversary, planDir string, phase string, planMD string, model string, iteration int) map[string]AdversaryResult {
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

			advResult, err := RunAdversary(ctx, a, planDir, phase, planMD, model, iteration)
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

	verdicts := make(map[string]AdversaryResult, len(adversaries))
	for r := range resultsCh {
		verdicts[r.Name] = r
	}
	return verdicts
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
