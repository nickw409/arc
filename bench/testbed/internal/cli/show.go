package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", args[0])
			}

			task, err := taskStore.Get(id)
			if err != nil {
				return err
			}

			fmt.Printf("Task #%d\n", task.ID)
			fmt.Printf("  Title:       %s\n", task.Title)
			fmt.Printf("  Status:      %s\n", task.Status)
			fmt.Printf("  Priority:    %s\n", task.Priority)
			fmt.Printf("  Created:     %s\n", task.CreatedAt.Format("2006-01-02 15:04:05"))
			if task.Description != "" {
				fmt.Printf("  Description: %s\n", task.Description)
			}
			if task.CompletedAt != nil {
				fmt.Printf("  Completed:   %s\n", task.CompletedAt.Format("2006-01-02 15:04:05"))
			}

			return nil
		},
	}
}
