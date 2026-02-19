package cli

import (
	"github.com/nwiley/arc/internal/selfupdate"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update Arc to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return selfupdate.Update(Version, "")
		},
	}
}
