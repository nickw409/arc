# Phase: optimize

## Objective

Fix the identified performance bottlenecks: add read caching to Store and replace O(n²) bubble sorts with O(n log n) `sort.Slice`. The existing `Add()` method calls `nextID()` which calls `load()` to read the file, then `Add()` calls `load()` again — eliminate this redundant read by caching.

## Files

### Modify
- `internal/store/store.go` — Add caching fields to Store, update `load()` to use cache, update `save()` to populate and validate cache, modify `Add()` to call `nextID()` only once and reuse the result

### Create
- `internal/store/cache_test.go` — Tests for cache correctness (TestCacheCorrectness, TestCacheUpdatedOnDelete)

## Types and Signatures

```go
// Modified Store struct (add exactly these two fields):
type Store struct {
    path       string
    cached     []model.Task  // NEW: cached result of last load
    cacheValid bool          // NEW: whether cache is current
}

// Modified constructor (update initialization):
func New(path string) *Store
// NEW behavior: Initialize cache fields:
//   return &Store{
//       path:       path,
//       cached:     nil,
//       cacheValid: false,
//   }

// Modified private methods (signatures unchanged):
func (s *Store) load() ([]model.Task, error)
// NEW behavior:
//   if s.cacheValid {
//       result := make([]model.Task, len(s.cached))
//       copy(result, s.cached)
//       return result, nil
//   }
//   // ... existing file read + unmarshal logic ...
//   s.cached = make([]model.Task, len(tasks))
//   copy(s.cached, tasks)
//   s.cacheValid = true
//   return tasks, nil

func (s *Store) save(tasks []model.Task) error
// NEW behavior: after writing file, update cache AND keep it valid:
//   // ... existing json.MarshalIndent + os.WriteFile logic ...
//   s.cached = make([]model.Task, len(tasks))
//   copy(s.cached, tasks)
//   s.cacheValid = true  // Cache is valid after save
//   return nil
// IMPORTANT: All mutation methods (Add, Delete, Update, Complete) must call save() as their final step.
// save() updates the cache with a fresh copy and sets cacheValid=true.
// DO NOT manually invalidate the cache in mutation methods — save() handles cache updates automatically.
// Verify each mutation method (Add, Delete, Update, Complete) ends with a call to save().

// Modified filter functions (signatures unchanged):
func SortByPriority(tasks []model.Task)
// DELETE the existing nested for-loop bubble sort implementation entirely.
// REPLACE with: sort.Slice(tasks, func(i, j int) bool { return tasks[i].Priority > tasks[j].Priority })
// This sorts descending by priority (higher priority values first).

func SortByDate(tasks []model.Task)
// DELETE the existing nested for-loop bubble sort implementation entirely.
// REPLACE with: sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.After(tasks[j].CreatedAt) })
// This sorts descending by date (newer tasks first).
```

## Error Types

No changes.

## Dependencies

None.

## DO NOT

- [ ] Do NOT change any public method signatures on Store
- [ ] Do NOT change the JSON file format
- [ ] Do NOT add external dependencies
- [ ] Do NOT change how the CLI calls the store — optimizations are internal
- [ ] Do NOT remove the benchmark tests from the baseline phase
- [ ] Do NOT break existing tests

## Test Cases

### Verify benchmarks improve
**Input:** Run `go test -bench=. ./internal/store/ ./internal/filter/`
**Expected:** Benchmarks execute without errors and show measurable improvement over baseline measurements. This is a verification that optimizations are working, not a pass/fail gate on specific performance targets.

### Verify all existing tests pass
**Input:** `go test ./...`
**Expected:** All tests pass — zero regressions

### TestCacheCorrectness
**Setup:** Create temp file store: `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`
**Input:**
1. `s.Add("Task A", "", model.PriorityLow)` → ID 1
2. `tasks1, _ := s.List()` → should return 1 task
3. `s.Add("Task B", "", model.PriorityHigh)` → ID 2
4. `tasks2, _ := s.List()` → should return 2 tasks
**Expected:** `len(tasks1) == 1`, `len(tasks2) == 2` — cache is properly updated on mutations

### TestNewStoreInitializesCache
**Setup:** `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`
**Input:** Inspect internal state after construction
**Expected:** `s.cached == nil`, `s.cacheValid == false`

### TestLoadUsesCache
**Setup:** Create temp file store, add one task to populate cache
**Input:**
1. `s.Add("Task A", "", model.PriorityLow)` → populates cache via save
2. `s.List()` → first call reads from cache
3. Modify the file on disk to add corrupted data
4. `s.List()` → second call should still return cached data without reading corrupted file
**Expected:** Second List() returns same valid data because cache is used (cacheValid == true)

### TestLoadPopulatesCache
**Setup:** Create temp file store
**Input:**
1. Manually write valid JSON to store file: `[{"id":1,"title":"Test","description":"","priority":1,"status":"pending","createdAt":"2026-01-01T00:00:00Z"}]`
2. `tasks, _ := s.List()` → first load from file
3. `len(tasks)` → should be 1
**Expected:** After first load, `s.cacheValid == true` and `s.cached` contains the loaded task

### TestSaveUpdatesCache
**Setup:** Create temp file store, add one task
**Input:**
1. `s.Add("Task A", "", model.PriorityLow)`
2. Inspect internal state
**Expected:** After Add (which calls save), `s.cacheValid == true` and `s.cached` contains the new task

### TestSaveErrorDoesNotUpdateCache
**Setup:** Create store with invalid path (e.g., `/invalid/nonexistent/path/tasks.json`)
**Input:**
1. Attempt `s.Add("Task A", "", model.PriorityLow)`
**Expected:** Add returns error, `s.cacheValid == false` — cache is not updated on save failure

### TestCacheUpdatedOnDelete
**Setup:** Create temp file store: `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 3 tasks: `s.Add("Task 1", "", model.PriorityLow)`, `s.Add("Task 2", "", model.PriorityMedium)`, `s.Add("Task 3", "", model.PriorityHigh)`
**Input:**
1. `s.List()` → 3 tasks (populates cache)
2. `s.Delete(2)` → removes task with ID 2
3. `s.List()` → must return 2 tasks (cache updated via save)
**Expected:** Second `List()` returns 2 tasks, task with ID 2 is no longer present

### TestSortSliceCorrectness
**Setup:** Create 5 tasks with priorities: Low(1), High(3), Medium(2), High(3), Low(1)
**Input:** Call `filter.SortByPriority(tasks)`
**Expected:** Tasks sorted by priority descending: High, High, Medium, Low, Low (priorities: 3, 3, 2, 1, 1)

### TestSortByPriorityEmptySlice
**Setup:** Create empty task slice: `tasks := []model.Task{}`
**Input:** Call `filter.SortByPriority(tasks)`
**Expected:** No panic, `len(tasks) == 0`

### TestSortByPrioritySingleTask
**Setup:** Create slice with one task: `tasks := []model.Task{{ID: 1, Priority: model.PriorityHigh}}`
**Input:** Call `filter.SortByPriority(tasks)`
**Expected:** No panic, task unchanged

### TestSortByPriorityAllSame
**Setup:** Create 3 tasks all with `model.PriorityMedium`
**Input:** Call `filter.SortByPriority(tasks)`
**Expected:** No panic, order may vary but all priorities remain Medium

### TestSortByDateEmptySlice
**Setup:** Create empty task slice: `tasks := []model.Task{}`
**Input:** Call `filter.SortByDate(tasks)`
**Expected:** No panic, `len(tasks) == 0`

### TestSortByDateSingleTask
**Setup:** Create slice with one task with CreatedAt set to now
**Input:** Call `filter.SortByDate(tasks)`
**Expected:** No panic, task unchanged

## Edge Cases

1. **Cache must return copies** — If `load()` returns the cached slice directly, callers could mutate the cache. Always use `make` + `copy`.
2. **Save updates cache** — After any mutation (Add, Delete, Update, Complete), the mutation method calls `save()`, which updates `s.cached` with a copy and keeps `s.cacheValid = true`, so subsequent reads use the cache.
3. **Mutation methods must call save** — All mutation methods (Add, Delete, Update, Complete) must call `save()` to persist changes and update the cache in one operation.
4. **Sort stability** — `sort.Slice` is not stable; if stable sort is needed, use `sort.SliceStable`. For this codebase, stability is not required.
5. **Empty task list** — Cache should work correctly with zero tasks (`s.cached` is `[]model.Task{}`, `s.cacheValid` is `true`)
6. **Save error invalidates cache** — If `os.WriteFile` fails in `save()`, `s.cacheValid` must remain false (or be set to false) to prevent serving stale cache data. Cache should only be updated AFTER successful file write.
7. **Load error does not corrupt cache** — If `json.Unmarshal` fails in `load()`, the error should be returned and `s.cacheValid` should remain false. Do not partially update the cache on error.
8. **Cache initialization** — New Store must initialize `cached` to `nil` and `cacheValid` to `false`

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase baseline: Benchmark tests must exist to measure improvement

### Exports
- Optimized Store implementation with read caching (internal changes only, no API changes)
