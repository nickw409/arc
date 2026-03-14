package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newWaitCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "wait <plan-name>",
		Short: "Block until a plan completes or any phase is blocked",
		Long:  "Polls phase state every 3 seconds and exits when all phases are terminal (complete/blocked/deferred/split). Exit code 0 if all phases completed, 1 if any phase is blocked.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			plansDir := filepath.Join(".plans", "active")

			opts := plan.StatusOptions{
				PlansDir: plansDir,
				PlanName: planName,
			}

			// Verify plan exists.
			planDir := filepath.Join(plansDir, planName)
			if _, err := os.Stat(filepath.Join(planDir, "plan.json")); err != nil {
				return fmt.Errorf("plan %q not found", planName)
			}

			// Already done?
			if plan.AllPhasesTerminal(opts) {
				return reportResult(plansDir, planName)
			}

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sig)

			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			var deadline <-chan time.Time
			if timeout > 0 {
				deadline = time.After(timeout)
			}

			for {
				select {
				case <-sig:
					fmt.Println()
					return nil
				case <-deadline:
					return fmt.Errorf("timeout waiting for plan %q", planName)
				case <-ticker.C:
					if plan.AllPhasesTerminal(opts) {
						return reportResult(plansDir, planName)
					}
				}
			}
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Maximum time to wait (e.g. 30m, 1h). 0 means wait forever.")
	return cmd
}

// reportResult prints a one-line summary and returns an error if any phase is blocked.
func reportResult(plansDir, planName string) error {
	summary, hasBlocked := plan.WaitSummary(plansDir, planName)
	fmt.Println(summary)
	if hasBlocked {
		return fmt.Errorf("plan %q has blocked phases", planName)
	}
	return nil
}
