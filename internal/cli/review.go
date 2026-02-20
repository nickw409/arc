package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/review"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <plan-name>",
		Short: "Review a plan before execution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			plansDir := filepath.Join(".plans", "active")

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

			overallStatus := "approved"

			for _, phase := range meta.Phases {
				fmt.Printf("Reviewing phase: %s\n", phase)

				result, err := review.Run(context.Background(), review.ReviewOptions{
					PlanName: planName,
					PlansDir: plansDir,
					ArcHome:  arcHome,
					Phase:    phase,
					Logger:   logger,
				})
				if err != nil {
					return fmt.Errorf("review phase %s: %w", phase, err)
				}

				for _, v := range result.Verdicts {
					statusMark := "PASS"
					if v.Status == "failed" || v.Status == "error" {
						statusMark = "FAIL"
					} else if v.Status == "cached" {
						statusMark = "CACHED"
					}
					fmt.Printf("  [%s] %s: verdict=%s\n", statusMark, v.Name, v.Verdict)
				}
				fmt.Printf("  Phase result: %s\n\n", result.Status)

				if result.Status == "needs_review" {
					overallStatus = "needs_review"
				} else if result.Status == "conditional" && overallStatus == "approved" {
					overallStatus = "conditional"
				}
			}

			fmt.Printf("Review complete: status=%s\n", overallStatus)
			return nil
		},
	}
}
