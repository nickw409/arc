// hidden_test.go — Validation tests for Task 2 (Bugfix)
// Copy into testbed/internal/store/ after agent completes work.
// Run with: go test ./internal/store/ -run TestHiddenBugfix
package store

import (
	"path/filepath"
	"testing"

	"github.com/nwiley/tkit/internal/model"
)

func bugfixStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(filepath.Join(dir, "tasks.json"))
}

// Core fix: completing a task must not corrupt adjacent tasks.
func TestHiddenBugfixAdjacentPriority(t *testing.T) {
	s := bugfixStore(t)

	s.Add("A", "", model.PriorityLow)    // 1
	s.Add("B", "", model.PriorityHigh)   // 2
	s.Add("C", "", model.PriorityMedium) // 3

	s.Complete(2)

	c, _ := s.Get(3)
	if c.Priority != model.PriorityMedium {
		t.Errorf("task C priority corrupted: got %d, want %d", c.Priority, model.PriorityMedium)
	}
}

// Edge case: complete the LAST task — should not panic or corrupt.
func TestHiddenBugfixCompleteLastTask(t *testing.T) {
	s := bugfixStore(t)

	s.Add("First", "", model.PriorityLow)
	s.Add("Last", "", model.PriorityHigh)

	if err := s.Complete(2); err != nil {
		t.Fatal(err)
	}

	first, _ := s.Get(1)
	if first.Priority != model.PriorityLow {
		t.Errorf("first task priority corrupted: got %d", first.Priority)
	}
}

// Edge case: complete the FIRST task — the second should not be affected.
func TestHiddenBugfixCompleteFirstTask(t *testing.T) {
	s := bugfixStore(t)

	s.Add("First", "", model.PriorityLow)
	s.Add("Second", "", model.PriorityHigh)

	s.Complete(1)

	second, _ := s.Get(2)
	if second.Priority != model.PriorityHigh {
		t.Errorf("second task priority corrupted: got %d, want %d", second.Priority, model.PriorityHigh)
	}
}

// Verify completed task retains its original priority.
func TestHiddenBugfixCompletedTaskRetainsPriority(t *testing.T) {
	s := bugfixStore(t)

	s.Add("Important", "", model.PriorityHigh)
	s.Complete(1)

	task, _ := s.Get(1)
	if task.Priority != model.PriorityHigh {
		t.Errorf("completed task should retain priority: got %d, want %d", task.Priority, model.PriorityHigh)
	}
}

// Stress: complete many tasks, no corruption anywhere.
func TestHiddenBugfixBulkComplete(t *testing.T) {
	s := bugfixStore(t)

	priorities := []model.Priority{
		model.PriorityLow, model.PriorityHigh, model.PriorityMedium,
		model.PriorityHigh, model.PriorityLow, model.PriorityMedium,
	}

	for i, p := range priorities {
		s.Add("Task "+string(rune('A'+i)), "", p)
	}

	// Complete every other task
	s.Complete(1)
	s.Complete(3)
	s.Complete(5)

	// Verify uncompleted tasks retain their priorities
	for _, id := range []int{2, 4, 6} {
		task, err := s.Get(id)
		if err != nil {
			t.Fatalf("failed to get task %d: %v", id, err)
		}
		expected := priorities[id-1]
		if task.Priority != expected {
			t.Errorf("task %d priority corrupted: got %d, want %d", id, task.Priority, expected)
		}
	}
}

// Verify the original failing test still passes.
func TestHiddenBugfixOriginalFailingTest(t *testing.T) {
	s := bugfixStore(t)

	s.Add("Task A", "", model.PriorityLow)
	s.Add("Task B", "", model.PriorityHigh)
	s.Add("Task C", "", model.PriorityMedium)

	if err := s.Complete(2); err != nil {
		t.Fatal(err)
	}

	taskC, _ := s.Get(3)
	if taskC.Priority != model.PriorityMedium {
		t.Errorf("Task C priority = %d, want %d", taskC.Priority, model.PriorityMedium)
	}

	taskA, _ := s.Get(1)
	if taskA.Priority != model.PriorityLow {
		t.Errorf("Task A priority = %d, want %d", taskA.Priority, model.PriorityLow)
	}
}
