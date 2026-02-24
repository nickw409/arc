package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a task as completed",
		Aliases: []string{"done"},
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", args[0])
			}

			if err := taskStore.Complete(id); err != nil {
				return err
			}

			fmt.Printf("Completed task #%d\n", id)
			return nil
		},
	}
}
