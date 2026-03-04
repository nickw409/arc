package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/pipeline"
	"github.com/nwiley/arc/internal/resources"
	"github.com/spf13/cobra"
)

func newIterateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "iterate <plan-name> <phase-name>",
		Short: "Run a single iteration for a phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]
			plansDir := filepath.Join(".plans", "active")

			// Verify phase exists
			phaseDir := filepath.Join(plansDir, planName, "phases", phaseName)
			if _, err := os.Stat(phaseDir); os.IsNotExist(err) {
				return fmt.Errorf("phase %q not found in plan %q", phaseName, planName)
			}

			arcHome := resolveArcHome()

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("getting home directory: %w", err)
			}
			resolver := resources.NewResolver(projectRoot, homeDir)

			result := pipeline.RunState(context.Background(), logger, pipeline.IterateOptions{
				PlanName:  planName,
				PhaseName: phaseName,
				PlansDir:  plansDir,
				ArcHome:   arcHome,
				Resolver:  resolver,
			})

			if result.Err != nil {
				return fmt.Errorf("iteration failed (%s): %w", result.Action, result.Err)
			}

			fmt.Printf("Iteration complete: next_state=%s verdict=%s\n", result.NextState, result.Verdict)
			return nil
		},
	}
}
