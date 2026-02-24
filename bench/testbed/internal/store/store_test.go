package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/tkit/internal/model"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(filepath.Join(dir, "tasks.json"))
}

func TestAdd(t *testing.T) {
	s := tempStore(t)

	task, err := s.Add("Test task", "A description", model.PriorityMedium)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != 1 {
		t.Errorf("expected ID 1, got %d", task.ID)
	}
	if task.Title != "Test task" {
		t.Errorf("expected title 'Test task', got %q", task.Title)
	}
	if task.Status != model.StatusPending {
		t.Errorf("expected status pending, got %q", task.Status)
	}
	if task.Priority != model.PriorityMedium {
		t.Errorf("expected priority medium, got %d", task.Priority)
	}
}

func TestAddIncrementsID(t *testing.T) {
	s := tempStore(t)

	t1, _ := s.Add("First", "", model.PriorityLow)
	t2, _ := s.Add("Second", "", model.PriorityLow)
	t3, _ := s.Add("Third", "", model.PriorityLow)

	if t1.ID != 1 || t2.ID != 2 || t3.ID != 3 {
		t.Errorf("IDs should be sequential: got %d, %d, %d", t1.ID, t2.ID, t3.ID)
	}
}

func TestList(t *testing.T) {
	s := tempStore(t)

	s.Add("A", "", model.PriorityLow)
	s.Add("B", "", model.PriorityHigh)

	tasks, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestListEmpty(t *testing.T) {
	s := tempStore(t)

	tasks, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestGet(t *testing.T) {
	s := tempStore(t)

	s.Add("Find me", "", model.PriorityHigh)

	task, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Find me" {
		t.Errorf("expected 'Find me', got %q", task.Title)
	}
}

func TestGetNotFound(t *testing.T) {
	s := tempStore(t)

	_, err := s.Get(999)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestComplete(t *testing.T) {
	s := tempStore(t)

	s.Add("Do this", "", model.PriorityMedium)

	if err := s.Complete(1); err != nil {
		t.Fatal(err)
	}

	task, _ := s.Get(1)
	if task.Status != model.StatusCompleted {
		t.Errorf("expected completed, got %q", task.Status)
	}
	if task.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestDelete(t *testing.T) {
	s := tempStore(t)

	s.Add("Delete me", "", model.PriorityLow)

	if err := s.Delete(1); err != nil {
		t.Fatal(err)
	}

	tasks, _ := s.List()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := tempStore(t)

	err := s.Delete(999)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestUpdate(t *testing.T) {
	s := tempStore(t)

	s.Add("Original", "Desc", model.PriorityLow)

	newTitle := "Updated"
	newPriority := model.PriorityHigh
	if err := s.Update(1, &newTitle, nil, &newPriority, nil); err != nil {
		t.Fatal(err)
	}

	task, _ := s.Get(1)
	if task.Title != "Updated" {
		t.Errorf("expected 'Updated', got %q", task.Title)
	}
	if task.Priority != model.PriorityHigh {
		t.Errorf("expected high priority, got %d", task.Priority)
	}
}

func TestListByStatus(t *testing.T) {
	s := tempStore(t)

	s.Add("Pending task", "", model.PriorityLow)
	s.Add("Another pending", "", model.PriorityMedium)

	tasks, err := s.ListByStatus(model.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(tasks))
	}
}

func TestSearch(t *testing.T) {
	s := tempStore(t)

	s.Add("Buy groceries", "milk and eggs", model.PriorityLow)
	s.Add("Fix bug", "null pointer", model.PriorityHigh)

	results, err := s.Search("groceries")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestCount(t *testing.T) {
	s := tempStore(t)

	s.Add("A", "", model.PriorityLow)
	s.Add("B", "", model.PriorityLow)

	count, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	s1 := New(path)
	s1.Add("Persisted", "", model.PriorityHigh)

	s2 := New(path)
	tasks, err := s2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Persisted" {
		t.Error("task should persist across store instances")
	}
}

func TestCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	os.WriteFile(path, []byte("not json"), 0644)

	s := New(path)
	_, err := s.List()
	if err == nil {
		t.Error("expected error for corrupt file")
	}
}
