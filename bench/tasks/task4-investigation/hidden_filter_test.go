// hidden_filter_test.go — Validation tests for Task 4 filter optimizations
// Copy into testbed/internal/filter/ after agent completes work.
// Run with: go test ./internal/filter/ -run TestHiddenFilter
//           go test ./internal/filter/ -bench BenchmarkHiddenFilter
package filter

import (
	"fmt"
	"testing"
	"time"

	"github.com/nwiley/tkit/internal/model"
)

func generateFilterTasks(n int) []model.Task {
	tasks := make([]model.Task, n)
	for i := range tasks {
		tasks[i] = model.Task{
			ID:        i + 1,
			Title:     fmt.Sprintf("Task %d", i+1),
			Status:    model.StatusPending,
			Priority:  model.Priority((i % 3) + 1),
			CreatedAt: time.Now(),
		}
	}
	return tasks
}

func TestHiddenFilterCorrectness(t *testing.T) {
	tasks := generateFilterTasks(1000)
	status := model.StatusPending
	priority := model.PriorityHigh

	result := Apply(tasks, Options{Status: &status, Priority: &priority})

	for _, task := range result {
		if task.Status != model.StatusPending {
			t.Errorf("filtered task has wrong status: %s", task.Status)
		}
		if task.Priority != model.PriorityHigh {
			t.Errorf("filtered task has wrong priority: %d", task.Priority)
		}
	}
}

func TestHiddenFilterSortCorrectness(t *testing.T) {
	tasks := generateFilterTasks(100)
	SortByPriority(tasks)

	for i := 1; i < len(tasks); i++ {
		if tasks[i].Priority > tasks[i-1].Priority {
			t.Errorf("sort broken at index %d: %d > %d",
				i, tasks[i].Priority, tasks[i-1].Priority)
		}
	}
}

// --- Performance assertion tests ---
// These FAIL on unoptimized O(n²) bubble sort and PASS with O(n log n) sort.

func TestHiddenFilterSortByPriorityIsFast(t *testing.T) {
	tasks := generateFilterTasks(10000)

	start := time.Now()
	SortByPriority(tasks)
	elapsed := time.Since(start)

	// O(n²) bubble sort on 10000 elements: 100ms–500ms.
	// O(n log n) sort.Slice on 10000 elements: <5ms.
	if elapsed > 50*time.Millisecond {
		t.Errorf("SortByPriority(10000) took %v — should use O(n log n) sort algorithm (expected <50ms)", elapsed)
	}
}

func TestHiddenFilterSortByDateIsFast(t *testing.T) {
	tasks := generateFilterTasks(10000)

	start := time.Now()
	SortByDate(tasks)
	elapsed := time.Since(start)

	// O(n²) bubble sort on 10000 elements: 100ms–500ms.
	// O(n log n) sort.Slice on 10000 elements: <5ms.
	if elapsed > 50*time.Millisecond {
		t.Errorf("SortByDate(10000) took %v — should use O(n log n) sort algorithm (expected <50ms)", elapsed)
	}
}

func BenchmarkHiddenFilterApply5000(b *testing.B) {
	tasks := generateFilterTasks(5000)
	status := model.StatusPending
	opts := Options{Status: &status, Query: "Task"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Apply(tasks, opts)
	}
}

func BenchmarkHiddenSortByPriority5000(b *testing.B) {
	base := generateFilterTasks(5000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tasks := make([]model.Task, len(base))
		copy(tasks, base)
		b.StartTimer()
		SortByPriority(tasks)
	}
}

func BenchmarkHiddenSortByDate5000(b *testing.B) {
	base := generateFilterTasks(5000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tasks := make([]model.Task, len(base))
		copy(tasks, base)
		b.StartTimer()
		SortByDate(tasks)
	}
}
