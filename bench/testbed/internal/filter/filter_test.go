package filter

import (
	"testing"
	"time"

	"github.com/nwiley/tkit/internal/model"
)

func makeTasks() []model.Task {
	now := time.Now()
	return []model.Task{
		{ID: 1, Title: "Buy groceries", Status: model.StatusPending, Priority: model.PriorityLow, CreatedAt: now},
		{ID: 2, Title: "Fix bug", Status: model.StatusActive, Priority: model.PriorityHigh, CreatedAt: now.Add(time.Minute)},
		{ID: 3, Title: "Write docs", Status: model.StatusPending, Priority: model.PriorityMedium, CreatedAt: now.Add(2 * time.Minute)},
		{ID: 4, Title: "Buy milk", Status: model.StatusCompleted, Priority: model.PriorityLow, CreatedAt: now.Add(3 * time.Minute)},
	}
}

func TestFilterByStatus(t *testing.T) {
	tasks := makeTasks()
	status := model.StatusPending
	result := Apply(tasks, Options{Status: &status})

	if len(result) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(result))
	}
}

func TestFilterByPriority(t *testing.T) {
	tasks := makeTasks()
	priority := model.PriorityHigh
	result := Apply(tasks, Options{Priority: &priority})

	if len(result) != 1 {
		t.Errorf("expected 1 high priority task, got %d", len(result))
	}
	if result[0].Title != "Fix bug" {
		t.Errorf("expected 'Fix bug', got %q", result[0].Title)
	}
}

func TestFilterByQuery(t *testing.T) {
	tasks := makeTasks()
	result := Apply(tasks, Options{Query: "buy"})

	if len(result) != 2 {
		t.Errorf("expected 2 tasks matching 'buy', got %d", len(result))
	}
}

func TestFilterCombined(t *testing.T) {
	tasks := makeTasks()
	status := model.StatusPending
	result := Apply(tasks, Options{Status: &status, Query: "buy"})

	if len(result) != 1 {
		t.Errorf("expected 1 pending+buy task, got %d", len(result))
	}
}

func TestFilterNoMatch(t *testing.T) {
	tasks := makeTasks()
	priority := model.PriorityHigh
	result := Apply(tasks, Options{Priority: &priority, Query: "nonexistent"})

	if len(result) != 0 {
		t.Errorf("expected 0 matches, got %d", len(result))
	}
}

func TestFilterEmpty(t *testing.T) {
	result := Apply(nil, Options{Query: "anything"})

	if len(result) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(result))
	}
}

func TestSortByPriority(t *testing.T) {
	tasks := makeTasks()
	SortByPriority(tasks)

	if tasks[0].Priority != model.PriorityHigh {
		t.Errorf("expected highest priority first, got %d", tasks[0].Priority)
	}
}

func TestSortByDate(t *testing.T) {
	tasks := makeTasks()
	SortByDate(tasks)

	if tasks[0].ID != 4 {
		t.Errorf("expected newest task first (ID 4), got ID %d", tasks[0].ID)
	}
}
