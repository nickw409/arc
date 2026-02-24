package cli

import (
	"fmt"

	"github.com/nwiley/tkit/internal/model"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		description string
		priority    string
	)

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := model.ParsePriority(priority)
			if err != nil {
				return err
			}

			task, err := taskStore.Add(args[0], description, p)
			if err != nil {
				return err
			}

			fmt.Printf("Created task #%d: %s\n", task.ID, task.Title)
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "desc", "d", "", "Task description")
	cmd.Flags().StringVarP(&priority, "priority", "p", "medium", "Priority (low, medium, high)")

	return cmd
}
