package store

import (
	"path/filepath"
	"testing"

	"github.com/nwiley/tkit/internal/model"
)

// TestCompleteDoesNotCorruptAdjacentTask verifies that completing a task
// does not modify any other task's data.
func TestCompleteDoesNotCorruptAdjacentTask(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "tasks.json"))

	// Create three tasks with distinct priorities
	s.Add("Task A", "", model.PriorityLow)    // ID 1
	s.Add("Task B", "", model.PriorityHigh)   // ID 2
	s.Add("Task C", "", model.PriorityMedium) // ID 3

	// Complete the middle task
	if err := s.Complete(2); err != nil {
		t.Fatal(err)
	}

	// Task C (ID 3) should be completely unaffected
	taskC, err := s.Get(3)
	if err != nil {
		t.Fatal(err)
	}

	if taskC.Priority != model.PriorityMedium {
		t.Errorf("Task C priority should be medium (%d), got %d (%s)",
			model.PriorityMedium, taskC.Priority, taskC.Priority)
	}

	if taskC.Status != model.StatusPending {
		t.Errorf("Task C status should be pending, got %s", taskC.Status)
	}

	// Task A should also be unaffected
	taskA, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}

	if taskA.Priority != model.PriorityLow {
		t.Errorf("Task A priority should be low (%d), got %d",
			model.PriorityLow, taskA.Priority)
	}
}
