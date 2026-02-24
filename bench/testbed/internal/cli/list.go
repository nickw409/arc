package cli

import (
	"fmt"

	"github.com/nwiley/tkit/internal/filter"
	"github.com/nwiley/tkit/internal/model"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var (
		status   string
		priority string
		query    string
		sortBy   string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := taskStore.List()
			if err != nil {
				return err
			}

			opts := filter.Options{Query: query}

			if status != "" {
				s, err := model.ParseStatus(status)
				if err != nil {
					return err
				}
				opts.Status = &s
			}
			if priority != "" {
				p, err := model.ParsePriority(priority)
				if err != nil {
					return err
				}
				opts.Priority = &p
			}

			tasks = filter.Apply(tasks, opts)

			switch sortBy {
			case "priority":
				filter.SortByPriority(tasks)
			case "date":
				filter.SortByDate(tasks)
			}

			if len(tasks) == 0 {
				fmt.Println("No tasks found.")
				return nil
			}

			for _, t := range tasks {
				statusIcon := " "
				switch t.Status {
				case model.StatusCompleted:
					statusIcon = "x"
				case model.StatusActive:
					statusIcon = ">"
				}
				fmt.Printf("[%s] #%d %s (priority: %s)\n", statusIcon, t.ID, t.Title, t.Priority)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status")
	cmd.Flags().StringVarP(&priority, "priority", "p", "", "Filter by priority")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Search title/description")
	cmd.Flags().StringVar(&sortBy, "sort", "", "Sort by: priority, date")

	return cmd
}
