// Package filter provides task filtering and sorting.
package filter

import (
	"strings"

	"github.com/nwiley/tkit/internal/model"
)

// Options specifies filter criteria for tasks.
type Options struct {
	Status   *model.Status
	Priority *model.Priority
	Query    string
}

// Apply filters a task list based on the given options.
// Each filter condition is applied as a separate pass over the list.
func Apply(tasks []model.Task, opts Options) []model.Task {
	result := tasks

	// Filter by status (separate pass)
	if opts.Status != nil {
		var filtered []model.Task
		for _, t := range result {
			if t.Status == *opts.Status {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	// Filter by priority (separate pass)
	if opts.Priority != nil {
		var filtered []model.Task
		for _, t := range result {
			if t.Priority == *opts.Priority {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	// Filter by query (separate pass)
	if opts.Query != "" {
		var filtered []model.Task
		q := strings.ToLower(opts.Query)
		for _, t := range result {
			if strings.Contains(strings.ToLower(t.Title), q) ||
				strings.Contains(strings.ToLower(t.Description), q) {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	return result
}

// SortByPriority sorts tasks by priority descending (high first).
func SortByPriority(tasks []model.Task) {
	n := len(tasks)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if tasks[j].Priority > tasks[i].Priority {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}

// SortByDate sorts tasks by creation date, newest first.
func SortByDate(tasks []model.Task) {
	n := len(tasks)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if tasks[j].CreatedAt.After(tasks[i].CreatedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}
