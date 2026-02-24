// hidden_test.go — Validation tests for Task 4 (Investigation/Performance)
// Copy into testbed/internal/store/ after agent completes work.
// Run with: go test ./internal/store/ -run TestHiddenPerf
//           go test ./internal/store/ -bench BenchmarkHidden -benchtime 3s
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/tkit/internal/model"
)

func generateTasks(n int) []model.Task {
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

func perfStore(t testing.TB, n int) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	tasks := generateTasks(n)
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	return New(path)
}

// --- Correctness after optimization ---

func TestHiddenPerfListStillCorrect(t *testing.T) {
	s := perfStore(t, 100)

	tasks, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 100 {
		t.Errorf("expected 100 tasks, got %d", len(tasks))
	}
}

func TestHiddenPerfCountStillCorrect(t *testing.T) {
	s := perfStore(t, 100)

	count, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Errorf("expected 100, got %d", count)
	}
}

func TestHiddenPerfSearchStillCorrect(t *testing.T) {
	s := perfStore(t, 100)

	results, err := s.Search("Task 50")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("search should find 'Task 50'")
	}
}

func TestHiddenPerfAddStillWorks(t *testing.T) {
	s := perfStore(t, 100)

	task, err := s.Add("New task", "", model.PriorityHigh)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != 101 {
		t.Errorf("expected ID 101, got %d", task.ID)
	}

	count, _ := s.Count()
	if count != 101 {
		t.Errorf("expected 101, got %d", count)
	}
}

func TestHiddenPerfCompleteStillWorks(t *testing.T) {
	s := perfStore(t, 100)

	if err := s.Complete(50); err != nil {
		t.Fatal(err)
	}

	task, _ := s.Get(50)
	if task.Status != model.StatusCompleted {
		t.Errorf("expected completed, got %s", task.Status)
	}
}

func TestHiddenPerfStatsPatternStillCorrect(t *testing.T) {
	s := perfStore(t, 200)

	// Simulate what stats command does
	total, _ := s.Count()
	pending, _ := s.CountByStatus(model.StatusPending)
	active, _ := s.CountByStatus(model.StatusActive)
	completed, _ := s.CountByStatus(model.StatusCompleted)

	if total != 200 {
		t.Errorf("total: expected 200, got %d", total)
	}
	if pending+active+completed != total {
		t.Errorf("status counts don't add up: %d+%d+%d != %d",
			pending, active, completed, total)
	}
}

// --- Performance assertion tests ---
// These FAIL on unoptimized code and PASS after proper optimization.
// They validate that the store caches file contents instead of re-reading
// the JSON file on every method call.

func TestHiddenPerfStoreRepeatedReadsAreFast(t *testing.T) {
	s := perfStore(t, 5000)

	// Time a single List() call (includes first-time I/O + parse cost)
	start := time.Now()
	s.List()
	singleCall := time.Since(start)

	// Time 100 more List() calls
	start = time.Now()
	for i := 0; i < 100; i++ {
		s.List()
	}
	hundredCalls := time.Since(start)

	// With caching, 100 calls return cached data (~0ms total).
	// Without caching, 100 calls re-read and parse 5000-task JSON 100 times.
	// Allow 10x the single-call time (floor 50ms) for 100 calls.
	threshold := singleCall * 10
	if threshold < 50*time.Millisecond {
		threshold = 50 * time.Millisecond
	}

	if hundredCalls > threshold {
		t.Errorf("100 List() calls took %v (single: %v, ratio: %.0fx) — "+
			"store should cache data instead of re-reading file on every call",
			hundredCalls, singleCall, float64(hundredCalls)/float64(singleCall))
	}
}

func TestHiddenPerfStoreStatsPatternIsFast(t *testing.T) {
	s := perfStore(t, 5000)

	// Prime with one call
	s.List()

	// The stats command calls Count + 3x CountByStatus = 4 methods per iteration.
	// Without caching, each method re-reads + parses the file.
	// 50 iterations × 4 methods = 200 file reads without caching.
	start := time.Now()
	for i := 0; i < 50; i++ {
		s.Count()
		s.CountByStatus(model.StatusPending)
		s.CountByStatus(model.StatusActive)
		s.CountByStatus(model.StatusCompleted)
	}
	elapsed := time.Since(start)

	// With caching: <10ms (just scanning cached slice).
	// Without caching: 200 parses of 5000-task JSON = 500ms–2s.
	if elapsed > 500*time.Millisecond {
		t.Errorf("50 stats iterations took %v — store should cache file contents "+
			"to avoid re-reading on every method call (expected <500ms)", elapsed)
	}
}

// --- Performance benchmarks ---
// These verify the optimization actually improved things.
// The threshold is generous — we just need to see it's not doing
// N file reads for N operations.

func BenchmarkHiddenList5000(b *testing.B) {
	s := perfStore(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.List()
	}
}

func BenchmarkHiddenStatsPattern5000(b *testing.B) {
	s := perfStore(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This is what the stats command does
		s.Count()
		s.CountByStatus(model.StatusPending)
		s.CountByStatus(model.StatusActive)
		s.CountByStatus(model.StatusCompleted)
	}
}

func BenchmarkHiddenSequentialOps5000(b *testing.B) {
	s := perfStore(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.List()
		s.Count()
		s.Search("Task 100")
		s.ListByStatus(model.StatusPending)
	}
}
