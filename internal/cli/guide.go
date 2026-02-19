package cli

import (
	"fmt"

	"github.com/nwiley/arc/internal/guide"
	"github.com/spf13/cobra"
)

func newGuideCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "guide [topic]",
		Short:     "Print the Arc reference guide for AI agents",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: guide.ValidSections(),
		RunE: func(cmd *cobra.Command, args []string) error {
			var section string
			if len(args) == 1 {
				section = args[0]
			}

			data, err := guide.Render(section)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}
