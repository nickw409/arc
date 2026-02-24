// hidden_test.go — Validation tests for Task 1 (Label/Tag Support)
// Copy into testbed/internal/store/ after agent completes work.
// Run with: go test ./internal/store/ -run TestHidden
package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nwiley/tkit/internal/model"
)

func hiddenStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(filepath.Join(dir, "tasks.json"))
}

// --- Core: Labels field exists and persists ---

func TestHiddenLabelsFieldExists(t *testing.T) {
	s := hiddenStore(t)
	task, err := s.Add("Test", "", model.PriorityLow)
	if err != nil {
		t.Fatal(err)
	}
	// Labels should be empty by default, not nil in behavior
	if task.Labels == nil {
		// nil is acceptable as long as it marshals correctly
	}

	// Reload and verify
	got, err := s.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 0 {
		t.Errorf("new task should have 0 labels, got %d", len(got.Labels))
	}
}

func TestHiddenLabelsPersistedOnAdd(t *testing.T) {
	s := hiddenStore(t)

	// The task struct should support labels.
	// We test by adding a label after creation and verifying persistence.
	task, _ := s.Add("Labeled task", "", model.PriorityMedium)

	err := s.AddLabel(task.ID, "work")
	if err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}

	got, _ := s.Get(task.ID)
	if !slices.Contains(got.Labels, "work") {
		t.Errorf("expected label 'work', got %v", got.Labels)
	}
}

// --- Label normalization ---

func TestHiddenLabelNormalization(t *testing.T) {
	s := hiddenStore(t)
	task, _ := s.Add("Test", "", model.PriorityLow)

	// Labels should be normalized to lowercase
	s.AddLabel(task.ID, "URGENT")
	got, _ := s.Get(task.ID)

	for _, l := range got.Labels {
		if l != strings.ToLower(l) {
			t.Errorf("label should be lowercase, got %q", l)
		}
	}
}

// --- No duplicate labels ---

func TestHiddenNoDuplicateLabels(t *testing.T) {
	s := hiddenStore(t)
	task, _ := s.Add("Test", "", model.PriorityLow)

	s.AddLabel(task.ID, "bug")
	s.AddLabel(task.ID, "bug") // duplicate

	got, _ := s.Get(task.ID)
	count := 0
	for _, l := range got.Labels {
		if l == "bug" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("should not have duplicate labels, found %d 'bug' labels", count)
	}
}

// --- Remove label ---

func TestHiddenRemoveLabel(t *testing.T) {
	s := hiddenStore(t)
	task, _ := s.Add("Test", "", model.PriorityLow)

	s.AddLabel(task.ID, "remove-me")
	s.AddLabel(task.ID, "keep-me")

	err := s.RemoveLabel(task.ID, "remove-me")
	if err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	got, _ := s.Get(task.ID)
	if slices.Contains(got.Labels, "remove-me") {
		t.Error("label 'remove-me' should have been removed")
	}
	if !slices.Contains(got.Labels, "keep-me") {
		t.Error("label 'keep-me' should still be present")
	}
}

// --- List all labels ---

func TestHiddenListAllLabels(t *testing.T) {
	s := hiddenStore(t)

	t1, _ := s.Add("Task 1", "", model.PriorityLow)
	t2, _ := s.Add("Task 2", "", model.PriorityLow)

	s.AddLabel(t1.ID, "bug")
	s.AddLabel(t1.ID, "frontend")
	s.AddLabel(t2.ID, "bug")
	s.AddLabel(t2.ID, "backend")

	labels, err := s.ListLabels()
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 unique labels: bug, frontend, backend
	if len(labels) != 3 {
		t.Errorf("expected 3 unique labels, got %d: %v", len(labels), labels)
	}

	for _, want := range []string{"bug", "frontend", "backend"} {
		if !slices.Contains(labels, want) {
			t.Errorf("missing label %q in %v", want, labels)
		}
	}
}

// --- Filter by label ---

func TestHiddenListByLabel(t *testing.T) {
	s := hiddenStore(t)

	t1, _ := s.Add("Bug task", "", model.PriorityHigh)
	s.Add("No labels", "", model.PriorityLow)
	t3, _ := s.Add("Another bug", "", model.PriorityMedium)

	s.AddLabel(t1.ID, "bug")
	s.AddLabel(t3.ID, "bug")

	results, err := s.ListByLabel("bug")
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 tasks with 'bug' label, got %d", len(results))
	}
}

// --- Existing tests still pass ---

func TestHiddenExistingAddStillWorks(t *testing.T) {
	s := hiddenStore(t)

	task, err := s.Add("Normal task", "desc", model.PriorityHigh)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != 1 {
		t.Errorf("expected ID 1, got %d", task.ID)
	}
	if task.Status != model.StatusPending {
		t.Errorf("expected pending, got %s", task.Status)
	}
}

func TestHiddenExistingCompleteStillWorks(t *testing.T) {
	s := hiddenStore(t)
	s.Add("Complete me", "", model.PriorityMedium)

	if err := s.Complete(1); err != nil {
		t.Fatal(err)
	}

	task, _ := s.Get(1)
	if task.Status != model.StatusCompleted {
		t.Errorf("expected completed, got %s", task.Status)
	}
}
