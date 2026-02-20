package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/review"
	"github.com/spf13/cobra"
)

const defaultReviewModel = "claude-sonnet-4-5-20250929"

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <plan-name>",
		Short: "Review a plan before execution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			plansDir := filepath.Join(".plans", "active")

			model, _ := cmd.Flags().GetString("model")
			if model == "" {
				model = defaultReviewModel
			}

			arcHome := os.Getenv("ARC_HOME")
			if arcHome == "" {
				ex, err := os.Executable()
				if err == nil {
					arcHome = filepath.Dir(filepath.Dir(ex))
				}
			}

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

			// Clear history if --reset flag is set
			reset, _ := cmd.Flags().GetBool("reset")
			if reset {
				histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
				if phaseFilter != "" {
					// Clear only the target phase from history
					history := review.LoadHistory(histPath)
					delete(history, phaseFilter)
					review.SaveHistory(histPath, history)
					logger.Info("reset review history", "phase", phaseFilter)
				} else {
					// Clear entire history file
					os.Remove(histPath)
					logger.Info("reset review history", "plan", planName)
				}
			}

			// Run phases in batches to avoid overwhelming the system
			const maxConcurrentPhases = 3

			type phaseResult struct {
				Phase  string
				Result *review.ReviewResult
				Err    error
			}

			resultsCh := make(chan phaseResult, len(phases))
			sem := make(chan struct{}, maxConcurrentPhases)

			var wg sync.WaitGroup
			for _, phase := range phases {
				wg.Add(1)
				go func(p string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					result, err := review.Run(context.Background(), review.ReviewOptions{
						PlanName: planName,
						PlansDir: plansDir,
						ArcHome:  arcHome,
						Phase:    p,
						Model:    model,
						Logger:   logger,
					})
					resultsCh <- phaseResult{Phase: p, Result: result, Err: err}
				}(phase)
			}

			wg.Wait()
			close(resultsCh)

			// Collect results keyed by phase
			phaseResults := make(map[string]phaseResult, len(phases))
			for r := range resultsCh {
				phaseResults[r.Phase] = r
			}

			// Print in phase order
			overallStatus := "approved"
			for _, phase := range phases {
				pr := phaseResults[phase]
				fmt.Printf("Phase: %s\n", phase)

				if pr.Err != nil {
					fmt.Printf("  ERROR: %v\n\n", pr.Err)
					overallStatus = "needs_review"
					continue
				}

				for _, v := range pr.Result.Verdicts {
					statusMark := "PASS"
					if v.Status == "failed" || v.Status == "error" {
						statusMark = "FAIL"
					} else if v.Status == "cached" && v.CachedStatus == "passed" {
						statusMark = "CACHED"
					} else if v.Status == "cached" {
						statusMark = "FAIL*"
					}
					fmt.Printf("  [%s] %s: verdict=%s\n", statusMark, v.Name, v.Verdict)
				}
				fmt.Printf("  Phase result: %s\n\n", pr.Result.Status)

				if pr.Result.Status == "needs_review" {
					overallStatus = "needs_review"
				} else if pr.Result.Status == "conditional" && overallStatus == "approved" {
					overallStatus = "conditional"
				}
			}

			fmt.Printf("Review complete: status=%s\n", overallStatus)
			return nil
		},
	}

	cmd.Flags().String("model", "", "Model to use for review agents (default: claude-sonnet-4-5-20250929)")
	cmd.Flags().String("phase", "", "Review a single phase instead of all phases")
	cmd.Flags().Bool("reset", false, "Clear cached review history before running")

	return cmd
}
