package cli

import "github.com/spf13/cobra"

func newIterateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "iterate",
		Short: "Run a single iteration",
	}
}
