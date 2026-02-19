package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

			result, err := review.Run(context.Background(), review.ReviewOptions{
				PlanName: planName,
				PlansDir: plansDir,
				ArcHome:  arcHome,
				Logger:   logger,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Review complete: status=%s\n", result.Status)
			return nil
		},
	}
}
