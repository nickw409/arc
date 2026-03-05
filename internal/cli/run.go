package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var timeout int
	var useWorktree bool
	var perPhaseWorktree bool
	var foreground bool
	var detached bool

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

			// Detach mode: re-exec ourselves with --detached and return immediately.
			if !foreground && !detached {
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("resolving executable path: %w", err)
				}

				reExecArgs := []string{"run", planName, "--detached"}
				if timeout != 14400 {
					reExecArgs = append(reExecArgs, fmt.Sprintf("--timeout=%d", timeout))
				}
				if !useWorktree {
					reExecArgs = append(reExecArgs, "--worktree=false")
				}
				if perPhaseWorktree {
					reExecArgs = append(reExecArgs, "--per-phase-worktree")
				}

				logPath := filepath.Join(planDir, "orchestrator.log")
				logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					return fmt.Errorf("creating log file: %w", err)
				}

				detachedCmd := exec.Command(exe, reExecArgs...)
				detachedCmd.Stdout = logFile
				detachedCmd.Stderr = logFile
				detachedCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

				if err := detachedCmd.Start(); err != nil {
					logFile.Close()
					return fmt.Errorf("starting detached orchestrator: %w", err)
				}
				logFile.Close()

				pidPath := filepath.Join(planDir, "orchestrator.pid")
				if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", detachedCmd.Process.Pid)), 0644); err != nil {
					return fmt.Errorf("writing PID file: %w", err)
				}

				fmt.Printf("Orchestrator started (PID %d)\n", detachedCmd.Process.Pid)
				fmt.Printf("Log: %s\n", logPath)
				fmt.Println("Use 'arc status' to check progress.")
				return nil
			}

			// Inline execution path (--foreground or --detached).
			pidPath := filepath.Join(planDir, "orchestrator.pid")
			if detached {
				// Write our own PID so status can track us.
				if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
					// Non-fatal; log and continue.
					fmt.Fprintf(os.Stderr, "warning: could not write PID file: %v\n", err)
				}
				defer os.Remove(pidPath)
			}

			// Resolve ARC_HOME
			arcHome := resolveArcHome()

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

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("getting home directory: %w", err)
			}
			resolver := resources.NewResolver(projectRoot, homeDir)

			_, err = orchestrator.Launch(ctx, orchestrator.LaunchOptions{
				PlanName:         planName,
				PlansDir:         plansDir,
				ArcHome:          arcHome,
				ProjectDir:       projectRoot,
				Config:           cfg,
				ConfigPath:       filepath.Join(projectRoot, ".arc.yaml"),
				Logger:           logger,
				Timeout:          timeout,
				UseWorktree:      useWorktree,
				PerPhaseWorktree: perPhaseWorktree,
				Resolver:         resolver,
			})
			return err
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 14400, "Wall-clock timeout in seconds")
	cmd.Flags().BoolVar(&useWorktree, "worktree", true, "Run agents in isolated git worktrees")
	cmd.Flags().BoolVar(&perPhaseWorktree, "per-phase-worktree", false, "Create a separate worktree per phase instead of one shared worktree")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run orchestrator in the foreground (blocking)")
	cmd.Flags().BoolVar(&detached, "detached", false, "")
	if err := cmd.Flags().MarkHidden("detached"); err != nil {
		panic(err)
	}
	return cmd
}
