package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/state"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var timeout int
	var useWorktree bool

	cmd := &cobra.Command{
		Use:   "run [plan-name]",
		Short: "Run the orchestrator for a plan",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(projectRoot)
			if err != nil {
				return fmt.Errorf("loading .arc.yaml: %w", err)
			}

			plansDir := filepath.Join(projectRoot, ".plans", "active")

			// Resolve plan name
			planName := ""
			if len(args) > 0 {
				planName = args[0]
			} else {
				entries, err := os.ReadDir(plansDir)
				if err != nil {
					return fmt.Errorf("no active plans found")
				}
				var plans []string
				for _, e := range entries {
					if e.IsDir() {
						plans = append(plans, e.Name())
					}
				}
				if len(plans) == 0 {
					return fmt.Errorf("no active plans found")
				}
				if len(plans) == 1 {
					planName = plans[0]
				} else {
					fmt.Println("Active plans:")
					for i, p := range plans {
						fmt.Printf("  %d. %s\n", i+1, p)
					}
					return fmt.Errorf("multiple plans found, specify one: arc run <plan-name>")
				}
			}

			// Verify plan exists
			planDir := filepath.Join(plansDir, planName)
			if _, err := os.Stat(planDir); os.IsNotExist(err) {
				return fmt.Errorf("plan %q not found at %s", planName, planDir)
			}

			// Verify plan is reviewed
			meta, err := state.ReadPlan(planDir)
			if err != nil {
				return fmt.Errorf("reading plan: %w", err)
			}
			if meta.ReviewStatus != "approved" && meta.ReviewStatus != "conditional" {
				return fmt.Errorf("plan %q has review status %q — run: arc review %s", planName, meta.ReviewStatus, planName)
			}

			// Resolve ARC_HOME
			arcHome := os.Getenv("ARC_HOME")
			if arcHome == "" {
				// Try to find it relative to the binary
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

			_, err = orchestrator.Launch(ctx, orchestrator.LaunchOptions{
				PlanName:    planName,
				PlansDir:    plansDir,
				ArcHome:     arcHome,
				ProjectDir:  projectRoot,
				Config:      cfg,
				Logger:      logger,
				Timeout:     timeout,
				UseWorktree: useWorktree,
			})
			return err
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 14400, "Wall-clock timeout in seconds")
	cmd.Flags().BoolVar(&useWorktree, "worktree", true, "Run agents in isolated git worktrees")
	return cmd
}
