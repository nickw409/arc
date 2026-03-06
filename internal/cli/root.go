package cli

import "github.com/spf13/cobra"

var Version = "dev"

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
		newStatusCmd(),
		newMonitorCmd(),
		newUpdateCmd(),
		newValidateCmd(),
		newGuideCmd(),
		newArchiveCmd(),
		newManageCmd(),
		newDevCmd(),
		newTaskCmd(),
		newServeCmd(),
		newChatCmd(),
		newTestCmd(),
		newGateCmd(),
		newAuditCmd(),
		newRecipeCmd(),
		newCancelCmd(),
		newCleanupCmd(),
		newDaemonCmd(),
	)
	return cmd
}
