# Phase: baseline

## Objective

Create Go benchmark tests that measure store and filter performance with 5,000 tasks, establishing a baseline before optimization.

## Files

### Create
- `internal/store/bench_test.go` — Benchmarks for Store operations
  - Package declaration: `package store`
  - Required imports: `"encoding/json"`, `"fmt"`, `"os"`, `"path/filepath"`, `"testing"`, `"time"`, and the model package (determine correct module path by reading `go.mod` at repository root, then import as `"<module_path>/internal/model"`)
- `internal/filter/bench_test.go` — Benchmarks for filter and sort operations
  - Package declaration: `package filter`
  - Required imports: `"fmt"`, `"testing"`, `"time"`, and the model package (determine correct module path by reading `go.mod` at repository root, then import as `"<module_path>/internal/model"`)

## Types and Signatures

```go
// Helper function in store/bench_test.go — named `makeBenchTasks` to avoid
// collisions with other test files in the same package:
func makeBenchTasks(n int) []model.Task {
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

// Helper function in filter/bench_test.go — named `makeBenchTasks` to match store package convention:
func makeBenchTasks(n int) []model.Task {
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

// In store/bench_test.go:
func BenchmarkStoreList(b *testing.B)
// Setup: Create temp dir via b.TempDir(), seed file with 5000 tasks via direct JSON write:
//   tempDir := b.TempDir()
//   path := filepath.Join(tempDir, "tasks.json")
//   tasks := makeBenchTasks(5000)
//   data, err := json.MarshalIndent(tasks, "", "  ")
//   if err != nil { b.Fatal(err) }
//   if err := os.WriteFile(path, data, 0644); err != nil { b.Fatal(err) }
//   s := New(path)  // New is in the same package (store), no qualifier needed
//   b.ResetTimer()  // AFTER all setup, BEFORE the b.N loop
// Benchmark loop: for i := 0; i < b.N; i++ { s.List() }
// This measures the cost of loading + unmarshaling the file on each call
// Required additional import: "path/filepath"

func BenchmarkStoreCount(b *testing.B)
// Setup: Same as BenchmarkStoreList:
//   tempDir := b.TempDir()
//   path := filepath.Join(tempDir, "tasks.json")
//   tasks := makeBenchTasks(5000)
//   data, err := json.MarshalIndent(tasks, "", "  ")
//   if err != nil { b.Fatal(err) }
//   if err := os.WriteFile(path, data, 0644); err != nil { b.Fatal(err) }
//   s := New(path)
//   b.ResetTimer()
// Benchmark loop: for i := 0; i < b.N; i++ { s.Count() }
// This measures the cost of loading + counting

func BenchmarkStoreStatsPattern(b *testing.B)
// Setup: Same as BenchmarkStoreList:
//   tempDir := b.TempDir()
//   path := filepath.Join(tempDir, "tasks.json")
//   tasks := makeBenchTasks(5000)
//   data, err := json.MarshalIndent(tasks, "", "  ")
//   if err != nil { b.Fatal(err) }
//   if err := os.WriteFile(path, data, 0644); err != nil { b.Fatal(err) }
//   s := New(path)
//   b.ResetTimer()
// Benchmark loop: for i := 0; i < b.N; i++ {
//   s.Count()
//   s.CountByStatus(model.StatusPending)
//   s.CountByStatus(model.StatusActive)
//   s.CountByStatus(model.StatusCompleted)
// }
// This simulates what the stats command does — 4 separate file reads per iteration

// In filter/bench_test.go:
func BenchmarkFilterApply(b *testing.B)
// Setup: Generate 5000 tasks with mixed statuses:
//   tasks := makeBenchTasks(5000)
//   // Assign ~1/3 StatusActive so the filter has realistic work to do:
//   for i := range tasks {
//       switch i % 3 {
//       case 0: tasks[i].Status = model.StatusPending
//       case 1: tasks[i].Status = model.StatusActive
//       case 2: tasks[i].Status = model.StatusCompleted
//       }
//   }
//   b.ResetTimer()
// Benchmark loop: for i := 0; i < b.N; i++ {
//   active := model.StatusActive
//   filter.Apply(tasks, filter.Options{Status: &active})
// }

func BenchmarkSortByPriority(b *testing.B)
// Setup: Generate 5000 tasks: tasks := makeBenchTasks(5000); b.ResetTimer()
// Benchmark loop: for i := 0; i < b.N; i++ {
//   cp := make([]model.Task, len(tasks))
//   copy(cp, tasks)
//   filter.SortByPriority(cp)
// }
// IMPORTANT: Copy before each sort so the benchmark measures unsorted input

func BenchmarkSortByDate(b *testing.B)
// Setup: Generate 5000 tasks: tasks := makeBenchTasks(5000); b.ResetTimer()
// Benchmark loop: for i := 0; i < b.N; i++ {
//   cp := make([]model.Task, len(tasks))
//   copy(cp, tasks)
//   filter.SortByDate(cp)
// }
```

## Error Types

None — benchmark-only phase.

## Dependencies

None.

## DO NOT

- [ ] Do NOT modify any production code — only add benchmark test files
- [ ] Do NOT modify existing test files
- [ ] Do NOT use `b.ResetTimer()` before the setup is done — use it AFTER setup
- [ ] Do NOT benchmark with fewer than 5000 tasks — the spec requires 5000

## Test Cases

### BenchmarkStoreList
**Setup:** Temp file with 5000 tasks serialized as JSON via `json.MarshalIndent(tasks, "", "  ")` then `os.WriteFile(path, data, 0644)`. Call `b.ResetTimer()` after setup.
**Measure:** Time for `s.List()` in `b.N` loop — expect slow due to file read on every call
**Record:** ns/op as baseline

### BenchmarkStoreCount
**Setup:** Same as BenchmarkStoreList (5000 tasks in JSON file, `b.ResetTimer()` after setup)
**Measure:** Time for `s.Count()` in `b.N` loop
**Record:** ns/op as baseline

### BenchmarkStoreStatsPattern
**Setup:** Same temp file, `b.ResetTimer()` after setup
**Measure:** Time for `Count() + CountByStatus(Pending) + CountByStatus(Active) + CountByStatus(Completed)` in `b.N` loop — expect ~4x slower than single List since it reads the file 4 times
**Record:** ns/op as baseline

### BenchmarkStoreGet
**Setup:** Temp file with 5000 tasks via JSON write (same as BenchmarkStoreList), `b.ResetTimer()` after setup
**Measure:** Time for `s.Get(2500)` in `b.N` loop — retrieves task from middle of dataset
**Record:** ns/op as baseline

### BenchmarkStoreUpdate
**Setup:** Temp file with 5000 tasks via JSON write, retrieve task 2500 via `task, _ := s.Get(2500)`, modify its title: `task.Title = "Updated"`, `b.ResetTimer()` after setup
**Measure:** Time for `s.Update(task)` in `b.N` loop — measures cost of read + unmarshal + find + replace + marshal + write
**Record:** ns/op as baseline

### BenchmarkStoreComplete
**Setup:** Temp file with 5000 tasks via JSON write, `b.ResetTimer()` after setup
**Measure:** Time for `s.Complete(2500)` in `b.N` loop — measures status transition cost
**Record:** ns/op as baseline

### BenchmarkStoreDelete
**Setup:** Temp file with 5000 tasks via JSON write, `b.ResetTimer()` after setup
**Measure:** Time for `s.Delete(2500)` in `b.N` loop — measures deletion cost (will eventually fail when task is gone, but measures initial deletions)
**Record:** ns/op as baseline

### BenchmarkStoreAdd
**Setup:** Create temp dir via `b.TempDir()`, initialize empty store with empty JSON array `[]` via `os.WriteFile(path, []byte("[]"), 0644)`, create a single task to add: `task := model.Task{Title: "Bench", Status: model.StatusPending, Priority: model.PriorityMedium, CreatedAt: time.Now()}`, `b.ResetTimer()` after setup
**Measure:** Time for `s.Add(task)` in `b.N` loop — measures write cost (re-reading entire file + unmarshaling + appending + marshaling + writing)
**Record:** ns/op as baseline

### BenchmarkFilterApply
**Setup:** Generate 5000 tasks in memory via `makeBenchTasks(5000)`, assign statuses deterministically using modulo 3 (i%3: 0→Pending, 1→Active, 2→Completed), `b.ResetTimer()` after setup
**Measure:** Time for `filter.Apply(tasks, Options{Status: &active})` in `b.N` loop — filters to exactly 1667 active tasks (indices where i%3==1)
**Record:** ns/op as baseline

### BenchmarkFilterApplyEmpty
**Setup:** Generate empty slice `tasks := []model.Task{}`, `b.ResetTimer()` after setup
**Measure:** Time for `filter.Apply(tasks, Options{Status: &active})` in `b.N` loop — edge case: filtering empty input
**Record:** ns/op as baseline

### BenchmarkFilterApplyPriority
**Setup:** Generate 5000 tasks with `makeBenchTasks(5000)` (priorities cycle 1→2→3 via modulo), `b.ResetTimer()` after setup
**Measure:** Time for `filter.Apply(tasks, Options{Priority: &high})` where `high := model.PriorityHigh` in `b.N` loop — filters to ~1667 high-priority tasks
**Record:** ns/op as baseline

### BenchmarkFilterApplyCombined
**Setup:** Generate 5000 tasks with mixed status (i%3 determines status as in BenchmarkFilterApply), `b.ResetTimer()` after setup
**Measure:** Time for `filter.Apply(tasks, Options{Status: &active, Priority: &high})` where `active := model.StatusActive` and `high := model.PriorityHigh` in `b.N` loop — tests combined filter performance
**Record:** ns/op as baseline

### BenchmarkSortByPriority
**Setup:** 5000 in-memory tasks via `makeBenchTasks(5000)`, `b.ResetTimer()` after setup
**Measure:** Time for copy + `SortByPriority(copy)` in `b.N` loop — expect slow due to O(n²) bubble sort
**Record:** ns/op as baseline

### BenchmarkSortByDate
**Setup:** 5000 in-memory tasks via `makeBenchTasks(5000)`, `b.ResetTimer()` after setup
**Measure:** Time for copy + `SortByDate(copy)` in `b.N` loop — expect slow due to O(n²) bubble sort
**Record:** ns/op as baseline

## Edge Cases

1. **Benchmark setup must write tasks via JSON directly** — Using Store.Add() 5000 times would be extremely slow (5000 file reads + writes)
2. **Sort benchmarks must copy input** — Sorting modifies the slice in place; re-sorting an already-sorted slice is O(n) for bubble sort, hiding the real cost
3. **Use `b.ResetTimer()` after setup** — Setup time should not be included in benchmark results
4. **Empty input benchmarks** — BenchmarkFilterApplyEmpty tests filtering with 0 tasks to ensure no panics and measure baseline overhead
5. **Write operation benchmarks** — BenchmarkStoreAdd measures per-operation write cost; note that Add() starts from empty file since pre-loading would skew results (the bottleneck is re-reading the growing file on each Add)

## Integration Points

### Consumed by
- Phase optimize: Baseline numbers are compared against post-optimization benchmarks

### Depends on
- Nothing — first phase

### Exports
- Benchmark test files that will be re-run after optimization to measure improvement
