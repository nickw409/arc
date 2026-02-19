package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/monitor"
	"github.com/spf13/cobra"
)

func newMonitorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "monitor [plan-name]",
		Short: "Monitor orchestrator progress",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir := filepath.Join(".plans", "active")

			var planName string
			if len(args) > 0 {
				planName = args[0]
			} else {
				entries, err := os.ReadDir(plansDir)
				if err != nil {
					return fmt.Errorf("reading plans: %w", err)
				}
				var dirs []string
				for _, e := range entries {
					if e.IsDir() {
						dirs = append(dirs, e.Name())
					}
				}
				if len(dirs) == 0 {
					return fmt.Errorf("no active plans found")
				}
				if len(dirs) > 1 {
					return fmt.Errorf("multiple plans found, specify one: %v", dirs)
				}
				planName = dirs[0]
			}

			return monitor.Start(monitor.StartOptions{
				PlanName: planName,
				PlansDir: plansDir,
			})
		},
	}
}
