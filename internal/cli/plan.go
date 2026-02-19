package cli

import "github.com/spf13/cobra"

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Manage plans",
	}
}
