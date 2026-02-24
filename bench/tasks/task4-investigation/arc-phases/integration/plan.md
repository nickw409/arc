# Phase: integration

## Objective

Write integration tests that verify caching and sort optimizations work correctly end-to-end under realistic usage patterns.

## Files

### Create
- `internal/store/perf_integration_test.go` — Integration tests for cache correctness and sort behavior

## Types and Signatures

```go
// No new production types. Tests use existing types:
// store.New(path string) *Store
// (*Store).Add(title string, description string, priority model.Priority) (*model.Task, error)
// (*Store).List() ([]model.Task, error)
// (*Store).Get(id int) (*model.Task, error)
// (*Store).Delete(id int) error
// (*Store).Update(id int, title *string, description *string, priority *model.Priority, status *model.Status) error
// (*Store).Complete(id int) error
// (*Store).Count() (int, error)
// filter.SortByPriority(tasks []model.Task)
// filter.SortByDate(tasks []model.Task)

// Test helper pattern:
// dir := t.TempDir()
// s := store.New(filepath.Join(dir, "tasks.json"))

// String pointer helper for Update calls (define at top of test file):
func strPtr(s string) *string { return &s }
```

## Error Types

None — test-only phase.

## Dependencies

- `testing` (standard library)
- `time` (standard library, for TestIntegrationSortCorrectness)
- `path/filepath` (standard library, for temp file paths)
- Existing project packages: `internal/store`, `internal/model`, `internal/filter`

## DO NOT

- [ ] Do NOT modify any production code — only add test files
- [ ] Do NOT modify existing test files or benchmark files
- [ ] Do NOT duplicate unit tests from the optimize phase (TestCacheCorrectness, TestCacheInvalidationOnDelete, TestSortSliceCorrectness)
- [ ] Do NOT modify `internal/store/store.go` or `internal/filter/filter.go`

## Test Cases

### TestIntegrationCacheCoherenceAfterError
**Setup:** Create temp file store with invalid/inaccessible path after initial setup
**Input:**
1. `s := store.New(filepath.Join(dir, "tasks.json"))` → normal store
2. `task1, err := s.Add("Task 1", "desc", model.PriorityHigh)` → verify err == nil, task1.ID == 1
3. `tasks, err := s.List()` → verify len(tasks) == 1, cache populated
4. Make the file read-only using `os.Chmod(filepath.Join(dir, "tasks.json"), 0444)` to simulate save failure
5. `task2, err := s.Add("Task 2", "desc2", model.PriorityLow)` → verify err != nil (save fails)
6. `tasks, err = s.List()` → verify len(tasks) == 1, tasks[0].Title == "Task 1" (cache not corrupted by failed operation)
7. Restore file permissions using `os.Chmod(filepath.Join(dir, "tasks.json"), 0644)`
8. `task3, err := s.Add("Task 3", "desc3", model.PriorityMedium)` → verify err == nil, successful recovery
9. `tasks, err = s.List()` → verify len(tasks) == 2, contains Task 1 and Task 3, not Task 2
**Expected:** Cache is not updated when save operations fail. Failed operations do not corrupt cache state. System recovers after transient errors.

### TestIntegrationCacheCorrectness
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`
**Input:** Interleave mutation and read operations:
1. `task1, err1 := s.Add("Task A", "", model.PriorityLow)` → verify err1 == nil, task1.ID == 1
2. `tasks, err := s.List()` → verify len(tasks) == 1, err == nil
3. `task2, err2 := s.Add("Task B", "", model.PriorityHigh)` → verify err2 == nil, task2.ID == 2
4. `tasks, err = s.List()` → verify len(tasks) == 2, err == nil
5. `err = s.Delete(1)` → verify err == nil
6. `tasks, err = s.List()` → verify len(tasks) == 1, tasks[0].Title == "Task B", err == nil
7. `err = s.Update(2, strPtr("Updated B"), nil, nil, nil)` → verify err == nil
8. `task, err := s.Get(2)` → verify task.Title == "Updated B", err == nil
9. `err = s.Complete(2)` → verify err == nil
10. `tasks, err = s.List()` → verify len(tasks) == 1, tasks[0].Status == model.StatusCompleted, err == nil
11. `err = s.Get(999)` → verify err != nil (invalid ID)
12. `err = s.Delete(999)` → verify err != nil (non-existent task)
**Expected:** Every `List()` and `Get()` call returns the current state — cache never serves stale data. All error cases return non-nil errors.

### TestIntegrationUpdateNoOp
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`
**Input:**
1. `task1, err := s.Add("Task A", "Description A", model.PriorityHigh)` → verify err == nil, task1.ID == 1
2. `err = s.Update(1, nil, nil, nil, nil)` → verify err == nil (no-op update succeeds)
3. `task, err := s.Get(1)` → verify task.Title == "Task A", task.Description == "Description A", task.Priority == model.PriorityHigh, task.Status == model.StatusPending, err == nil
4. `tasks, err := s.List()` → verify len(tasks) == 1, tasks[0] matches task from step 3
**Expected:** Update with all nil parameters succeeds without modifying the task. Task fields remain unchanged. Cache is still functional after no-op update.

### TestIntegrationSortCorrectness
**Setup:** Create tasks in memory with varied priorities and dates:
```go
now := time.Now()
tasks := []model.Task{
    {ID: 1, Priority: model.PriorityLow, CreatedAt: now.Add(-3 * time.Hour)},
    {ID: 2, Priority: model.PriorityHigh, CreatedAt: now.Add(-1 * time.Hour)},
    {ID: 3, Priority: model.PriorityMedium, CreatedAt: now.Add(-2 * time.Hour)},
    {ID: 4, Priority: model.PriorityHigh, CreatedAt: now},
    {ID: 5, Priority: model.PriorityLow, CreatedAt: now.Add(-4 * time.Hour)},
}
```
**Input:** Sort by priority, then create a fresh slice with the same 5 tasks in original order and sort by date, then test edge cases
**Expected:**
- After `filter.SortByPriority(tasks)`: sorted in descending priority order (High before Medium before Low). Verify: tasks[0].Priority == model.PriorityHigh && tasks[1].Priority == model.PriorityHigh && tasks[2].Priority == model.PriorityMedium && tasks[3].Priority == model.PriorityLow && tasks[4].Priority == model.PriorityLow
- After creating fresh tasks slice and `filter.SortByDate(tasks)`: sorted by CreatedAt descending (newest first). Verify IDs in order: [4, 2, 3, 1, 5]
- `filter.SortByPriority([]model.Task{})` does not panic
- `filter.SortByDate([]model.Task{})` does not panic
- `filter.SortByPriority([]model.Task{{ID: 1, Priority: model.PriorityHigh, CreatedAt: now}})` does not panic, slice unchanged

### TestIntegrationCacheAcrossMethods
**Setup:** Create temp file store, add 10 tasks:
- Tasks 1-4: pending status, priority High, titles "Pending 1" through "Pending 4"
- Tasks 5-7: active status, priority Medium, titles "Active 1" through "Active 3"
- Tasks 8-10: completed status, priority Low, titles "Completed 1" through "Completed 3"
**Input:** Call in sequence:
1. `s.Count()` → returns 10
2. `s.List()` → returns 10 tasks
3. Loop through List() result to count pending tasks (status == model.StatusPending) → verify count == 4
4. Loop through List() result to count active tasks (status == model.StatusActive) → verify count == 3
5. Repeat `s.Count()` and `s.List()` — should be served from cache (results identical)
**Expected:** All methods return consistent data reflecting the same underlying state. No stale results.

### TestIntegrationCacheInvalidationSequence
**Setup:** Create temp file store
**Input:** Execute mutation sequence and verify cache invalidation:
1. `s.Add("Task 1", "", model.PriorityHigh)` → ID 1
2. `tasks, err := s.List()` → verify len(tasks) == 1, err == nil
3. `s.Update(1, strPtr("Updated"), nil, nil, nil)` → error == nil
4. `tasks, err = s.List()` → verify tasks[0].Title == "Updated", err == nil (cache invalidated)
5. `s.Complete(1)` → error == nil
6. `tasks, err = s.List()` → verify tasks[0].Status == model.StatusCompleted, err == nil (cache invalidated)
7. `s.Delete(1)` → error == nil
8. `tasks, err = s.List()` → verify len(tasks) == 0, err == nil (cache invalidated)
9. `err = s.Get(1)` → verify err != nil (deleted task)
10. `err = s.Complete(999)` → verify err != nil (non-existent task)
11. `err = s.Update(999, strPtr("Invalid"), nil, nil, nil)` → verify err != nil (non-existent task)
12. `err = s.Update(2, nil, nil, nil, nil)` → verify err == nil (no-op update on deleted task ID should return error since task was deleted in step 7)
**Expected:** Every mutation invalidates cache. Subsequent List() calls return updated data, not stale cached data. Operations on non-existent or deleted tasks return errors. Update with all nil params on non-existent task returns error.

## Edge Cases

1. **Cache coherence after error** — TestIntegrationCacheCoherenceAfterError covers read-only file scenario
2. **Empty store with cache** — Covered in TestIntegrationCacheInvalidationSequence step 8 (list after deleting only task)
3. **Sort with equal elements** — Tasks with same priority/date should not cause issues (sort.Slice handles ties)
4. **Sort empty list** — Covered in TestIntegrationSortCorrectness: `filter.SortByPriority([]model.Task{})` and `filter.SortByDate([]model.Task{})` should not panic
5. **Sort single element** — Covered in TestIntegrationSortCorrectness: Sorting a single-task slice should succeed without modification
6. **Get invalid ID** — Covered in TestIntegrationCacheCorrectness step 11 and TestIntegrationCacheInvalidationSequence step 9
7. **Delete non-existent task** — Covered in TestIntegrationCacheCorrectness step 12
8. **Update with all nil params** — Add to TestIntegrationCacheInvalidationSequence after step 11: `err = s.Update(1, nil, nil, nil, nil)` → verify err == nil, then verify task unchanged via Get()

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase baseline: Benchmark tests in `internal/store/bench_test.go` and `internal/filter/bench_test.go` must exist to measure improvement
- Phase optimize: Cache fields (`cached []model.Task`, `cacheValid bool`) and `sort.Slice` replacements must be implemented in `internal/store/store.go` and `internal/filter/filter.go`

### Exports
- None — this is the terminal phase
