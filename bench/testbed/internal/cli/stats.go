package cli

import (
	"fmt"

	"github.com/nwiley/tkit/internal/model"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show task statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			total, err := taskStore.Count()
			if err != nil {
				return err
			}

			pending, err := taskStore.CountByStatus(model.StatusPending)
			if err != nil {
				return err
			}

			active, err := taskStore.CountByStatus(model.StatusActive)
			if err != nil {
				return err
			}

			completed, err := taskStore.CountByStatus(model.StatusCompleted)
			if err != nil {
				return err
			}

			fmt.Printf("Total:     %d\n", total)
			fmt.Printf("Pending:   %d\n", pending)
			fmt.Printf("Active:    %d\n", active)
			fmt.Printf("Completed: %d\n", completed)

			return nil
		},
	}
}
