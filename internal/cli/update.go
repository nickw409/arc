package cli

import "github.com/spf13/cobra"

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update Arc",
	}
}
