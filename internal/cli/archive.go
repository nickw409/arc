package cli

import (
	"fmt"
	"path/filepath"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newArchiveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "archive <plan-name>",
		Short: "Archive a completed plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir := filepath.Join(".plans", "active")
			archiveDir := filepath.Join(".plans", "archive")

			err := plan.Archive(plan.ArchiveOptions{
				PlansDir:   plansDir,
				ArchiveDir: archiveDir,
				PlanName:   args[0],
				Force:      force,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Archived plan %q\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Archive even if phases are not all terminal")
	return cmd
}
