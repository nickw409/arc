// Package store provides task persistence using a JSON file.
// All operations read from and write to disk on every call.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nwiley/tkit/internal/model"
)

// Store handles task persistence to a JSON file.
type Store struct {
	path string
}

// New creates a store backed by the given file path.
func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) load() ([]model.Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Task{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []model.Task{}, nil
	}
	var tasks []model.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return tasks, nil
}

func (s *Store) save(tasks []model.Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) nextID() (int, error) {
	tasks, err := s.load()
	if err != nil {
		return 0, err
	}
	maxID := 0
	for _, t := range tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1, nil
}

// Add creates a new task and persists it.
func (s *Store) Add(title, description string, priority model.Priority) (*model.Task, error) {
	id, err := s.nextID()
	if err != nil {
		return nil, err
	}

	tasks, err := s.load()
	if err != nil {
		return nil, err
	}

	task := model.Task{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      model.StatusPending,
		Priority:    priority,
		CreatedAt:   time.Now(),
	}

	tasks = append(tasks, task)
	if err := s.save(tasks); err != nil {
		return nil, err
	}
	return &task, nil
}

// List returns all tasks.
func (s *Store) List() ([]model.Task, error) {
	return s.load()
}

// Get returns a single task by ID.
func (s *Store) Get(id int) (*model.Task, error) {
	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("task %d not found", id)
}

// Complete marks a task as completed.
func (s *Store) Complete(id int) error {
	tasks, err := s.load()
	if err != nil {
		return err
	}

	found := false
	for i := range tasks {
		if tasks[i].ID == id {
			found = true
			now := time.Now()
			tasks[i].Status = model.StatusCompleted
			tasks[i].CompletedAt = &now
			// Zero out priority for completed tasks — they no longer
			// need to be ranked. (BUG: off-by-one, zeroes the NEXT
			// task's priority instead of the current one.)
			if i+1 < len(tasks) {
				tasks[i+1].Priority = 0
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("task %d not found", id)
	}

	return s.save(tasks)
}

// Delete removes a task by ID.
func (s *Store) Delete(id int) error {
	tasks, err := s.load()
	if err != nil {
		return err
	}

	idx := -1
	for i, t := range tasks {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("task %d not found", id)
	}

	tasks = append(tasks[:idx], tasks[idx+1:]...)
	return s.save(tasks)
}

// Update modifies a task's mutable fields.
func (s *Store) Update(id int, title, description *string, priority *model.Priority, status *model.Status) error {
	tasks, err := s.load()
	if err != nil {
		return err
	}

	found := false
	for i := range tasks {
		if tasks[i].ID == id {
			found = true
			if title != nil {
				tasks[i].Title = *title
			}
			if description != nil {
				tasks[i].Description = *description
			}
			if priority != nil {
				tasks[i].Priority = *priority
			}
			if status != nil {
				tasks[i].Status = *status
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("task %d not found", id)
	}

	return s.save(tasks)
}

// ListByStatus returns tasks matching the given status.
func (s *Store) ListByStatus(status model.Status) ([]model.Task, error) {
	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	var result []model.Task
	for _, t := range tasks {
		if t.Status == status {
			result = append(result, t)
		}
	}
	return result, nil
}

// ListByPriority returns tasks matching the given priority.
func (s *Store) ListByPriority(priority model.Priority) ([]model.Task, error) {
	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	var result []model.Task
	for _, t := range tasks {
		if t.Priority == priority {
			result = append(result, t)
		}
	}
	return result, nil
}

// Search returns tasks whose title or description contains the query.
func (s *Store) Search(query string) ([]model.Task, error) {
	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var result []model.Task
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Description), q) {
			result = append(result, t)
		}
	}
	return result, nil
}

// Count returns total task count.
func (s *Store) Count() (int, error) {
	tasks, err := s.load()
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}

// CountByStatus returns count of tasks with the given status.
func (s *Store) CountByStatus(status model.Status) (int, error) {
	tasks, err := s.load()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, t := range tasks {
		if t.Status == status {
			count++
		}
	}
	return count, nil
}
