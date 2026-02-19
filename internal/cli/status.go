package cli

import (
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [plan-name]",
		Short: "Show current plan status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir := filepath.Join(".plans", "active")

			var planName string
			if len(args) > 0 {
				planName = args[0]
			}

			return plan.Status(os.Stdout, plan.StatusOptions{
				PlansDir: plansDir,
				PlanName: planName,
			})
		},
	}
}
