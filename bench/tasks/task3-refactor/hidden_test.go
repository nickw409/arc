// hidden_test.go — Validation tests for Task 3 (Refactor)
// Copy into testbed/internal/store/ after agent completes work.
// Run with: go test ./internal/store/ -run TestHiddenRefactor
package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nwiley/tkit/internal/model"
)

// --- Backend interface exists and is usable ---

func TestHiddenRefactorBackendInterfaceExists(t *testing.T) {
	// This test verifies the Backend interface exists by creating
	// a MemoryBackend and passing it to NewWithBackend.
	mb := &MemoryBackend{}
	s := NewWithBackend(mb)
	if s == nil {
		t.Fatal("NewWithBackend returned nil")
	}
}

// --- MemoryBackend works ---

func TestHiddenRefactorMemoryBackendRoundTrip(t *testing.T) {
	mb := &MemoryBackend{}

	tasks := []model.Task{
		{ID: 1, Title: "Test", Status: model.StatusPending, Priority: model.PriorityHigh},
	}

	if err := mb.Save(tasks); err != nil {
		t.Fatal(err)
	}

	loaded, err := mb.Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 1 || loaded[0].Title != "Test" {
		t.Errorf("MemoryBackend round-trip failed: got %v", loaded)
	}
}

func TestHiddenRefactorMemoryBackendIsolation(t *testing.T) {
	mb := &MemoryBackend{}

	tasks := []model.Task{
		{ID: 1, Title: "Original", Status: model.StatusPending},
	}
	mb.Save(tasks)

	// Modify the returned slice — should not affect stored data
	loaded, _ := mb.Load()
	loaded[0].Title = "Modified"

	reloaded, _ := mb.Load()
	if reloaded[0].Title != "Original" {
		t.Error("MemoryBackend should return copies, not references")
	}
}

// --- JSONBackend works ---

func TestHiddenRefactorJSONBackendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	jb := &JSONBackend{Path: filepath.Join(dir, "test.json")}

	tasks := []model.Task{
		{ID: 1, Title: "JSON test", Status: model.StatusActive, Priority: model.PriorityMedium},
	}

	if err := jb.Save(tasks); err != nil {
		t.Fatal(err)
	}

	loaded, err := jb.Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 1 || loaded[0].Title != "JSON test" {
		t.Errorf("JSONBackend round-trip failed: got %v", loaded)
	}
}

func TestHiddenRefactorJSONBackendMissingFile(t *testing.T) {
	dir := t.TempDir()
	jb := &JSONBackend{Path: filepath.Join(dir, "nonexistent.json")}

	tasks, err := jb.Load()
	if err != nil {
		t.Fatalf("missing file should return empty slice, got error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("missing file should return empty slice, got %d tasks", len(tasks))
	}
}

// --- Store works with MemoryBackend ---

func TestHiddenRefactorStoreWithMemoryBackend(t *testing.T) {
	s := NewWithBackend(&MemoryBackend{})

	task, err := s.Add("Memory task", "desc", model.PriorityHigh)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Title != "Memory task" {
		t.Errorf("expected 'Memory task', got %q", got.Title)
	}
}

func TestHiddenRefactorStoreAllOperationsWithMemoryBackend(t *testing.T) {
	s := NewWithBackend(&MemoryBackend{})

	// Add
	s.Add("A", "", model.PriorityLow)
	s.Add("B", "", model.PriorityHigh)
	s.Add("C", "", model.PriorityMedium)

	// List
	tasks, _ := s.List()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Complete
	s.Complete(2)
	task, _ := s.Get(2)
	if task.Status != model.StatusCompleted {
		t.Errorf("expected completed, got %s", task.Status)
	}

	// Delete
	s.Delete(1)
	tasks, _ = s.List()
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks after delete, got %d", len(tasks))
	}

	// Count
	count, _ := s.Count()
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Search
	results, _ := s.Search("C")
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}
}

// --- New(path) still works (backwards compatibility) ---

func TestHiddenRefactorNewPathStillWorks(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "tasks.json"))

	task, err := s.Add("Backwards compat", "", model.PriorityLow)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Title != "Backwards compat" {
		t.Errorf("New(path) should still work, got %q", got.Title)
	}
}

// --- Backend interface has correct shape ---

func TestHiddenRefactorBackendInterface(t *testing.T) {
	// Compile-time check that both backends implement the interface
	var _ Backend = (*JSONBackend)(nil)
	var _ Backend = (*MemoryBackend)(nil)

	// Verify Backend is an interface type
	bt := reflect.TypeOf((*Backend)(nil)).Elem()
	if bt.Kind() != reflect.Interface {
		t.Error("Backend should be an interface")
	}

	// Should have Load and Save methods
	if _, ok := bt.MethodByName("Load"); !ok {
		t.Error("Backend interface should have Load method")
	}
	if _, ok := bt.MethodByName("Save"); !ok {
		t.Error("Backend interface should have Save method")
	}
}
