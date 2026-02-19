package cli

import "github.com/spf13/cobra"

const Version = "0.2.0"

// NewRootCmd creates the root cobra command with all subcommands registered.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "arc",
		Short:   "Arc orchestration engine",
		Version: Version,
	}
	cmd.AddCommand(
		newInitCmd(),
		newRunCmd(),
		newPlanCmd(),
		newReviewCmd(),
		newIterateCmd(),
		newStatusCmd(),
		newMonitorCmd(),
		newUpdateCmd(),
		newValidateCmd(),
	)
	return cmd
}
