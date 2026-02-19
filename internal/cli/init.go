package cli

import (
	"fmt"

	"github.com/nwiley/arc/internal/project"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Arc project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := project.Init(project.InitOptions{
				ProjectRoot: ".",
				Force:       force,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized arc project (language=%s, runner=%s)\n", cfg.Language, cfg.Runner)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing .arc.yaml")

	return cmd
}
