package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var live bool

	cmd := &cobra.Command{
		Use:   "status [plan-name]",
		Short: "Show current plan status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir := filepath.Join(".plans", "active")

			var planName string
			if len(args) > 0 {
				planName = args[0]
			}

			opts := plan.StatusOptions{
				PlansDir: plansDir,
				PlanName: planName,
			}

			if !live {
				return plan.Status(os.Stdout, opts)
			}

			return runLiveStatus(opts)
		},
	}

	cmd.Flags().BoolVar(&live, "live", false, "Re-print status every 2 seconds until all phases are terminal or Ctrl+C is pressed")
	return cmd
}

// runLiveStatus clears the terminal and re-prints status every 2 seconds until
// all phases reach a terminal state (complete, blocked, or deferred) or the
// user presses Ctrl+C.
func runLiveStatus(opts plan.StatusOptions) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Print immediately on first iteration without waiting for the tick.
	if done, err := printLiveStatus(opts); err != nil {
		return err
	} else if done {
		return nil
	}

	for {
		select {
		case <-sig:
			fmt.Println()
			return nil
		case <-ticker.C:
			if done, err := printLiveStatus(opts); err != nil {
				return err
			} else if done {
				return nil
			}
		}
	}
}

// printLiveStatus clears the screen, renders status, and returns true when all
// phases are in a terminal state.
func printLiveStatus(opts plan.StatusOptions) (bool, error) {
	// Render into a buffer first so we clear the screen only on success.
	var buf bytes.Buffer
	if err := plan.Status(&buf, opts); err != nil {
		return false, err
	}

	// Clear terminal and move cursor to top-left.
	fmt.Print("\033[H\033[2J")
	os.Stdout.Write(buf.Bytes())

	return plan.AllPhasesTerminal(opts), nil
}
