package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/dev"
	"github.com/spf13/cobra"
)

func newDevCmd() *cobra.Command {
	var interactive bool
	var timeout int
	var skipReview bool
	var model string
	var autoYes bool

	cmd := &cobra.Command{
		Use:   "dev [task description...]",
		Short: "Analyze, plan, and execute a development task",
		Long:  "Spawns agents to analyze the task, generate a plan, optionally review it, then run the orchestrator.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDescription := strings.Join(args, " ")

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Load config (non-fatal if missing)
			cfg, _ := config.Load(projectRoot)

			// Ensure .plans/active directory exists
			plansDir := filepath.Join(projectRoot, ".plans", "active")
			if err := os.MkdirAll(plansDir, 0755); err != nil {
				return fmt.Errorf("creating plans directory: %w", err)
			}

			// Resolve ARC_HOME
			arcHome := os.Getenv("ARC_HOME")
			if arcHome == "" {
				ex, err := os.Executable()
				if err == nil {
					arcHome = filepath.Dir(filepath.Dir(ex))
				}
			}

			// Set up context with signal handling
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Fprintf(os.Stderr, "\nReceived interrupt, shutting down...\n")
				cancel()
			}()

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			result, err := dev.RunDev(ctx, dev.DevOptions{
				TaskDescription: taskDescription,
				ProjectDir:      projectRoot,
				ArcHome:         arcHome,
				Config:          cfg,
				Logger:          logger,
				Interactive:     interactive,
				Model:           model,
				Timeout:         timeout,
				SkipReview:      skipReview,
				AutoYes:         autoYes,
			})
			if err != nil {
				return err
			}

			fmt.Printf("\n[dev] Done. Plan: %s, Complexity: %s\n", result.PlanName, result.Complexity)
			if result.Usage.CostUSD > 0 {
				fmt.Printf("[dev] Cost: $%.4f\n", result.Usage.CostUSD)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&interactive, "interactive", false, "Pause at decision points")
	cmd.Flags().IntVar(&timeout, "timeout", 14400, "Wall-clock timeout in seconds")
	cmd.Flags().BoolVar(&skipReview, "skip-review", false, "Skip adversarial review")
	cmd.Flags().StringVar(&model, "model", "", "Model override for all agents")
	cmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Skip clarification questions (for CI)")
	return cmd
}
