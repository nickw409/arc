package cli

import (
	"fmt"
	"path/filepath"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	var (
		workflowType string
		role         string
	)

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

			if role != "" && role != "impl" && role != "review" && role != "investigate" && role != "audit" {
				return fmt.Errorf("role must be impl, review, investigate, or audit; got %q", role)
			}

			var phaseRoles map[string]string
			if role != "" {
				phaseRoles = make(map[string]string, len(phases))
				for _, p := range phases {
					phaseRoles[p] = role
				}
			}

			meta, err := plan.Create(plan.CreateOptions{
				PlansDir:     plansDir,
				Name:         name,
				Phases:       phases,
				WorkflowType: workflowType,
				PhaseRoles:   phaseRoles,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Created plan %q with %d phases\n", meta.Name, len(meta.Phases))
			return nil
		},
	}

	cmd.Flags().StringVar(&workflowType, "type", "feature", "Workflow type (feature, bugfix, investigation, refactor, performance, audit)")
	cmd.Flags().StringVar(&role, "role", "", "Phase Roles: impl, review, investigate, or audit (default: impl)")

	// Register spec subcommands
	addPlanSpecSubcommands(cmd)

	return cmd
}
