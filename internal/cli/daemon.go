package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/daemon"
	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the arc background daemon",
	}
	cmd.AddCommand(
		newDaemonStartCmd(),
		newDaemonStopCmd(),
		newDaemonStatusCmd(),
		newDaemonSubmitCmd(),
		newDaemonCancelCmd(),
	)
	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	var foreground bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the arc daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := daemon.LoadDaemonConfig()
			if err != nil {
				return err
			}

			if !foreground {
				// Auto-start as detached process.
				if err := daemon.EnsureRunning(cfg.SocketPath); err != nil {
					return err
				}
				fmt.Println("Daemon started.")
				return nil
			}

			// Foreground mode: run the daemon in this process.
			sched := daemon.NewScheduler(cfg.MaxParallel, daemon.DefaultPhaseRunner(), daemon.DefaultFinalizer())
			d := daemon.New(*cfg, sched)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle SIGTERM/SIGINT for graceful shutdown.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
			go func() {
				sig := <-sigCh
				slog.Info("received signal, shutting down", "signal", sig)
				cancel()
			}()

			// Run scheduler in background.
			go sched.Run(ctx)

			fmt.Printf("Arc daemon listening on %s (PID %d)\n", cfg.SocketPath, os.Getpid())
			return d.Start(ctx, func(conn net.Conn) {
				daemon.HandleConnection(conn, sched, cfg)
			})
		},
	}
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in foreground (don't detach)")
	return cmd
}

func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath := daemon.DefaultSocketPath()
			if !daemon.IsRunning(socketPath) {
				return fmt.Errorf("daemon is not running")
			}

			client, err := daemon.Connect(socketPath, 5*time.Second)
			if err != nil {
				return fmt.Errorf("connecting to daemon: %w", err)
			}
			defer client.Close()

			resp, err := client.Drain()
			if err != nil {
				return fmt.Errorf("sending drain: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("drain failed: %s", resp.Error)
			}

			fmt.Println("Daemon draining — will stop after current work completes.")
			return nil
		},
	}
}

func newDaemonStatusCmd() *cobra.Command {
	var planName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon and plan status",
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath := daemon.DefaultSocketPath()
			if !daemon.IsRunning(socketPath) {
				fmt.Println("Daemon is not running.")
				return nil
			}

			client, err := daemon.Connect(socketPath, 5*time.Second)
			if err != nil {
				return fmt.Errorf("connecting to daemon: %w", err)
			}
			defer client.Close()

			resp, err := client.Status(planName)
			if err != nil {
				return fmt.Errorf("querying status: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("status error: %s", resp.Error)
			}

			out, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&planName, "plan", "", "Show status for a specific plan")
	return cmd
}

func newDaemonSubmitCmd() *cobra.Command {
	var (
		useWorktree      bool
		perPhaseWorktree bool
		stopOnFailure    bool
		timeout          int
	)
	cmd := &cobra.Command{
		Use:   "submit <plan-name>",
		Short: "Submit a plan to the daemon for execution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath := daemon.DefaultSocketPath()
			if err := daemon.EnsureRunning(socketPath); err != nil {
				return fmt.Errorf("ensuring daemon is running: %w", err)
			}

			projectDir, err := os.Getwd()
			if err != nil {
				return err
			}

			client, err := daemon.Connect(socketPath, 5*time.Second)
			if err != nil {
				return fmt.Errorf("connecting to daemon: %w", err)
			}
			defer client.Close()

			resp, err := client.Submit(daemon.Request{
				Plan:             args[0],
				Project:          projectDir,
				Timeout:          timeout,
				UseWorktree:      useWorktree,
				PerPhaseWorktree: perPhaseWorktree,
				StopOnFailure:    stopOnFailure,
			})
			if err != nil {
				return fmt.Errorf("submitting plan: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("submit failed: %s", resp.Error)
			}

			fmt.Printf("Plan %q submitted (%d phases queued)\n", args[0], resp.QueuedPhases)
			return nil
		},
	}
	cmd.Flags().BoolVar(&useWorktree, "worktree", true, "Use git worktree isolation")
	cmd.Flags().BoolVar(&perPhaseWorktree, "per-phase-worktree", false, "Create a worktree per phase")
	cmd.Flags().BoolVar(&stopOnFailure, "stop-on-failure", false, "Stop plan on first phase failure")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "Plan timeout in seconds (0 = no timeout)")
	return cmd
}

func newDaemonCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <plan-name>",
		Short: "Cancel a plan running in the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath := daemon.DefaultSocketPath()
			if !daemon.IsRunning(socketPath) {
				return fmt.Errorf("daemon is not running")
			}

			client, err := daemon.Connect(socketPath, 5*time.Second)
			if err != nil {
				return fmt.Errorf("connecting to daemon: %w", err)
			}
			defer client.Close()

			resp, err := client.Cancel(args[0])
			if err != nil {
				return fmt.Errorf("cancelling plan: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("cancel failed: %s", resp.Error)
			}

			fmt.Printf("Plan %q cancelled.\n", args[0])
			return nil
		},
	}
}
