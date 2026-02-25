package cli

import (
	"fmt"
	"path/filepath"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	var workflowType string

	cmd := &cobra.Command{
		Use:   "plan <name> <phase1> [phase2...]",
		Short: "Create a new plan",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir := filepath.Join(".plans", "active")
			name := args[0]
			phases := args[1:]

			if workflowType == "" {
				workflowType = "feature"
			}

			meta, err := plan.Create(plan.CreateOptions{
				PlansDir:     plansDir,
				Name:         name,
				Phases:       phases,
				WorkflowType: workflowType,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Created plan %q with %d phases\n", meta.Name, len(meta.Phases))
			return nil
		},
	}

	cmd.Flags().StringVar(&workflowType, "type", "feature", "Workflow type (feature, bugfix, investigation, refactor, performance, audit)")
	return cmd
}
