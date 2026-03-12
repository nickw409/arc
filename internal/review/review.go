package review

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// MaxReviewIterations is the maximum number of review iterations per phase.
const MaxReviewIterations = 5

// ReviewOptions configures the adversarial review loop.
type ReviewOptions struct {
	PlanName       string
	PlansDir       string
	ArcHome        string
	Phase          string
	Model          string
	Logger         *slog.Logger
	MaxIterations  int    // if > 0, overrides MaxReviewIterations (default 5)
	ProjectContext string // language, build command, test command for adversaries
}

// ReviewResult holds the outcome of a review.
type ReviewResult struct {
	Status      string
	Verdicts    map[string]AdversaryResult
	Iteration   int
	Synthesized bool
	Usage       arc.Usage
	Hash        string // SHA-256 of the final plan.md content
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

// Run executes a single-pass adversarial review with synthesis.
// Runs scope first; if scope_too_large returns immediately.
// Otherwise runs spec-quality, correctness, gate in parallel, then synthesizer if any failed.
func Run(ctx context.Context, opts ReviewOptions) (*ReviewResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	planMDPath := filepath.Join(planDir, "phases", opts.Phase, "plan.md")
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	reviewsDir := filepath.Join(planDir, "reviews")
	if err := os.MkdirAll(reviewsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating reviews directory: %w", err)
	}

	result := &ReviewResult{
		Verdicts: make(map[string]AdversaryResult),
	}

	// Check iteration limit — if already at max, return conditional from cache.
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
		result.Status = "conditional"
		result.Iteration = history.Iterations[opts.Phase]
		for _, adv := range DefaultAdversaries() {
			if phaseHistory, ok := history.Phases[opts.Phase]; ok {
				if entry, ok := phaseHistory[adv.Name]; ok {
					result.Verdicts[adv.Name] = AdversaryResult{
						Name:         adv.Name,
						Verdict:      entry.Verdict,
						Status:       "cached",
						CachedStatus: entry.Status,
						Required:     adv.Required,
					}
				}
			}
		}
		if finalHash, err := computePlanHash(planMDPath); err == nil {
			result.Hash = finalHash
		}
		return result, nil
	}

	// Load plan.md.
	planMDBytes, err := os.ReadFile(planMDPath)
	if err != nil {
		return nil, fmt.Errorf("reading plan.md for phase %s: %w", opts.Phase, err)
	}
	planMD := string(planMDBytes)

	// --- Scope pre-check ---
	scopeAdv := ScopeAdversary()
	scopeResult, _ := RunAdversary(ctx, scopeAdv, planDir, opts.Phase, planMD, opts.Model, opts.ProjectContext)
	result.Usage = result.Usage.Add(scopeResult.Usage)
	result.Verdicts[scopeAdv.Name] = *scopeResult

	// Write scope output file and update history.
	hash, _ := computePlanHash(planMDPath)
	if history.Phases[opts.Phase] == nil {
		history.Phases[opts.Phase] = make(map[string]historyEntry)
	}
	scopeNonCached := scopeResult.Status != "cached" && scopeResult.Status != "error"
	if scopeNonCached {
		outPath := filepath.Join(reviewsDir, opts.Phase+"_"+scopeAdv.Name+".md")
		if err := os.WriteFile(outPath, []byte(scopeResult.Output), 0644); err != nil {
			opts.Logger.Warn("failed to write scope output file", "path", outPath, "error", err)
		}
		history.Phases[opts.Phase][scopeAdv.Name] = historyEntry{
			Hash:      hash,
			Verdict:   scopeResult.Verdict,
			Status:    scopeResult.Status,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}

	if scopeResult.Verdict == scopeAdv.FailVerdict {
		// Increment iteration once for the scope-only run.
		if scopeNonCached {
			history.Iterations[opts.Phase]++
		}
		if err := SaveHistory(histPath, history); err != nil {
			opts.Logger.Warn("failed to save adversary history", "error", err)
		}
		opts.Logger.Info("scope too large — plan must be split before continuing review",
			"phase", opts.Phase,
		)
		result.Status = "scope_too_large"
		result.Iteration = history.Iterations[opts.Phase]
		if finalHash, err := computePlanHash(planMDPath); err == nil {
			result.Hash = finalHash
		}
		return result, nil
	}

	// --- Run remaining adversaries in parallel ---
	var parallelAdvs []Adversary
	for _, adv := range DefaultAdversaries() {
		if adv.Name != scopeAdv.Name {
			parallelAdvs = append(parallelAdvs, adv)
		}
	}

	verdicts := runAdversaries(ctx, parallelAdvs, planDir, opts.Phase, planMD, opts.Model, opts.ProjectContext)

	// Merge scope result into verdicts.
	for k, v := range verdicts {
		result.Verdicts[k] = v
	}

	// Aggregate usage.
	for _, v := range verdicts {
		result.Usage = result.Usage.Add(v.Usage)
	}

	// Write output files and update history.
	// history is already loaded above and has the scope entry written.
	hasNonCached := scopeNonCached
	for _, v := range verdicts {
		if v.Status == "cached" || v.Status == "error" {
			continue
		}
		hasNonCached = true
		outPath := filepath.Join(reviewsDir, opts.Phase+"_"+v.Name+".md")
		if err := os.WriteFile(outPath, []byte(v.Output), 0644); err != nil {
			opts.Logger.Warn("failed to write adversary output file", "path", outPath, "error", err)
		}
		history.Phases[opts.Phase][v.Name] = historyEntry{
			Hash:      hash,
			Verdict:   v.Verdict,
			Status:    v.Status,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}
	// Increment iteration once for the whole run (scope + parallel).
	if hasNonCached {
		history.Iterations[opts.Phase]++
	}
	if err := SaveHistory(histPath, history); err != nil {
		opts.Logger.Warn("failed to save adversary history", "error", err)
	}

	status := determineReviewStatus(result.Verdicts)
	result.Iteration = history.Iterations[opts.Phase]
	result.Status = status

	opts.Logger.Info("review complete",
		"phase", opts.Phase,
		"iteration", result.Iteration,
		"status", status,
	)

	// Run synthesizer if any adversary failed.
	if status != "approved" {
		var failedNames []string
		for _, v := range result.Verdicts {
			effectiveStatus := v.Status
			if v.Status == "cached" {
				effectiveStatus = v.CachedStatus
			}
			if effectiveStatus == "failed" {
				failedNames = append(failedNames, v.Name)
			}
		}
		if len(failedNames) > 0 {
			synthesized, synthUsage, synthErr := RunSynthesizer(ctx, SynthesisOptions{
				PlanDir:         planDir,
				PhaseName:       opts.Phase,
				FailedCritiques: failedNames,
				Model:           opts.Model,
				CommandName:     agentCommandName,
				ProjectContext:  opts.ProjectContext,
			})
			result.Synthesized = synthesized
			result.Usage = result.Usage.Add(synthUsage)
			if synthErr != nil {
				opts.Logger.Warn("synthesizer failed (non-blocking)", "phase", opts.Phase, "error", synthErr)
			}
		}
	}

	// Downgrade needs_review to conditional — review is advisory, not blocking.
	if result.Status == "needs_review" {
		result.Status = "conditional"
	}

	if finalHash, err := computePlanHash(planMDPath); err == nil {
		result.Hash = finalHash
	}

	return result, nil
}

// runAdversaries runs adversaries concurrently and returns their results by name.
func runAdversaries(ctx context.Context, adversaries []Adversary, planDir string, phase string, planMD string, model string, projectContext string) map[string]AdversaryResult {
	type result struct {
		name string
		r    AdversaryResult
	}
	resultsCh := make(chan result, len(adversaries))

	var wg sync.WaitGroup
	for _, adv := range adversaries {
		wg.Add(1)
		go func(a Adversary) {
			defer wg.Done()
			r, _ := RunAdversary(ctx, a, planDir, phase, planMD, model, projectContext)
			resultsCh <- result{name: a.Name, r: *r}
		}(adv)
	}

	wg.Wait()
	close(resultsCh)

	verdicts := make(map[string]AdversaryResult, len(adversaries))
	for r := range resultsCh {
		verdicts[r.name] = r.r
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
