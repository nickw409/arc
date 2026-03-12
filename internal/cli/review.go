package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/project"
	"github.com/nwiley/arc/internal/review"
	"github.com/spf13/cobra"
)

const defaultReviewModel = "claude-sonnet-4-5-20250929"

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <plan-name>",
		Short: "Run adversarial review with synthesis pass",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			plansDir := filepath.Join(".plans", "active")

			model, _ := cmd.Flags().GetString("model")
			if model == "" {
				model = defaultReviewModel
			}

			arcHome := resolveArcHome()

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			// Read plan.json to discover phases
			planDir := filepath.Join(plansDir, planName)
			metaBytes, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
			if err != nil {
				return fmt.Errorf("reading plan.json: %w", err)
			}
			var meta arc.PlanMeta
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				return fmt.Errorf("parsing plan.json: %w", err)
			}

			// Filter to specific phase if requested
			phaseFilter, _ := cmd.Flags().GetString("phase")
			phases := meta.Phases
			if phaseFilter != "" {
				found := false
				for _, p := range meta.Phases {
					if p == phaseFilter {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("phase %q not found in plan", phaseFilter)
				}
				phases = []string{phaseFilter}
			}

			// Validate all phase specs before running any review agents.
			// Fatal warnings (e.g. malformed gate assertions) are hard errors —
			// fix them before review so adversaries don't waste time on a broken spec.
			var fatalWarnings []string
			for _, phaseName := range phases {
				spec, specErr := plan.ReadSpec(plansDir, planName, phaseName)
				if specErr != nil {
					fatalWarnings = append(fatalWarnings, fmt.Sprintf("phase %q — cannot read spec.yaml: %v", phaseName, specErr))
					continue
				}
				for _, w := range plan.ValidateSpec(spec) {
					if w.Fatal {
						fatalWarnings = append(fatalWarnings, fmt.Sprintf("phase %q — %s", phaseName, w))
					} else {
						fmt.Fprintf(os.Stderr, "warning: phase %q spec.yaml — %s\n", phaseName, w)
					}
				}
			}
			if len(fatalWarnings) > 0 {
				for _, msg := range fatalWarnings {
					fmt.Fprintf(os.Stderr, "error: spec.yaml %s\n", msg)
				}
				return fmt.Errorf("spec validation failed — fix spec.yaml errors before reviewing")
			}

			// Build project context for adversaries
			det := project.Detect(".")
			projectCtx := fmt.Sprintf("Language: %s\nBuild: %s\nTest: %s", det.Language, det.BuildCommand, det.TestCommand)

			// Determine which phases need review (skip unchanged if not force-targeted)
			forcePhase := phaseFilter != ""
			var phasesToReview []string
			for _, phase := range phases {
				planMDPath := filepath.Join(planDir, "phases", phase, "plan.md")
				currentHash, _ := review.ComputePlanHash(planMDPath)
				if !forcePhase && meta.PhaseReview != nil {
					if pr, ok := meta.PhaseReview[phase]; ok && pr.Hash == currentHash && currentHash != "" {
						if pr.Status == "approved" || pr.Status == "conditional" {
							fmt.Printf("Phase: %s (unchanged, skipped)\n", phase)
							continue
						}
					}
				}
				phasesToReview = append(phasesToReview, phase)
			}

			// Run phases in batches to avoid overwhelming the system
			const maxConcurrentPhases = 3

			type phaseResult struct {
				Phase  string
				Result *review.ReviewResult
				Err    error
			}

			resultsCh := make(chan phaseResult, len(phasesToReview))
			sem := make(chan struct{}, maxConcurrentPhases)

			var wg sync.WaitGroup
			for _, phase := range phasesToReview {
				wg.Add(1)
				go func(p string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					result, err := review.Run(context.Background(), review.ReviewOptions{
						PlanName:       planName,
						PlansDir:       plansDir,
						ArcHome:        arcHome,
						Phase:          p,
						Model:          model,
						Logger:         logger,
						ProjectContext: projectCtx,
					})
					resultsCh <- phaseResult{Phase: p, Result: result, Err: err}
				}(phase)
			}

			wg.Wait()
			close(resultsCh)

			// Collect results keyed by phase
			phaseResults := make(map[string]phaseResult, len(phasesToReview))
			for r := range resultsCh {
				phaseResults[r.Phase] = r
			}

			// Print in phase order (only reviewed phases)
			maxIteration := 0
			reviewResults := make(map[string]string)
			for _, phase := range phasesToReview {
				pr := phaseResults[phase]

				if pr.Err != nil {
					fmt.Printf("Phase: %s\n", phase)
					fmt.Printf("  ERROR: %v\n\n", pr.Err)
					continue
				}

				fmt.Printf("Phase: %s\n", phase)

				if pr.Result.Iteration > maxIteration {
					maxIteration = pr.Result.Iteration
				}

				// Show verdicts
				fmt.Printf("  Verdicts:\n")
				for _, v := range pr.Result.Verdicts {
					statusMark := "PASS"
					if v.Status == "failed" || v.Status == "error" {
						statusMark = "FAIL"
					} else if v.Status == "cached" && v.CachedStatus == "passed" {
						statusMark = "PASS (cached)"
					} else if v.Status == "cached" {
						statusMark = "FAIL (cached)"
					}
					fmt.Printf("    [%s] %s: %s\n", statusMark, v.Name, v.Verdict)
				}
				fmt.Printf("  Phase result: %s\n", pr.Result.Status)
				if pr.Result.Synthesized {
					fmt.Printf("  Synthesized: plan.md rewritten\n")
				}
				fmt.Println()

				for _, v := range pr.Result.Verdicts {
					effectiveStatus := v.Status
					if v.Status == "cached" {
						effectiveStatus = v.CachedStatus
					}
					reviewResults[v.Name] = effectiveStatus
				}
			}

			// Update plan.json with per-phase review status and recompute plan-level status
			metaBytes, err = os.ReadFile(filepath.Join(planDir, "plan.json"))
			if err == nil {
				var updatedMeta arc.PlanMeta
				if err := json.Unmarshal(metaBytes, &updatedMeta); err == nil {
					if updatedMeta.PhaseReview == nil {
						updatedMeta.PhaseReview = make(map[string]arc.PhaseReviewStatus)
					}
					now := time.Now().UTC().Format(time.RFC3339)
					for _, phase := range phasesToReview {
						pr := phaseResults[phase]
						if pr.Err == nil {
							updatedMeta.PhaseReview[phase] = arc.PhaseReviewStatus{
								Status:     pr.Result.Status,
								ReviewedAt: now,
								Hash:       pr.Result.Hash,
							}
						}
					}
					updatedMeta.ReviewStatus = computePlanReviewStatus(updatedMeta.PhaseReview, updatedMeta.Phases)
					updatedMeta.ReviewedAt = now
					updatedMeta.ReviewIterations = maxIteration
					updatedMeta.ReviewResults = reviewResults
					if data, err := json.MarshalIndent(updatedMeta, "", "  "); err == nil {
						os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)
					}
				}
			}

			fmt.Printf("Review complete: status=%s\n", computePlanReviewStatus(meta.PhaseReview, meta.Phases))

			return nil
		},
	}

	cmd.Flags().String("model", "", "Model to use for review agents (default: claude-sonnet-4-5-20250929)")
	cmd.Flags().String("phase", "", "Review a single phase instead of all phases")

	return cmd
}

// computePlanReviewStatus returns the worst-case status across all phases in the plan.
// Phases not yet reviewed are treated as "needs_review".
func computePlanReviewStatus(phaseReview map[string]arc.PhaseReviewStatus, phases []string) string {
	status := "approved"
	for _, p := range phases {
		pr, ok := phaseReview[p]
		if !ok || pr.Status == "needs_review" {
			return "needs_review"
		}
		if pr.Status == "conditional" {
			status = "conditional"
		}
	}
	return status
}
