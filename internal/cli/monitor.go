package cli

import "github.com/spf13/cobra"

func newMonitorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "monitor",
		Short: "Monitor orchestrator progress",
	}
}
