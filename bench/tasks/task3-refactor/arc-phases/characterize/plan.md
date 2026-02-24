# Phase: characterize

## Objective

Write characterization tests that capture the current behavior of the Store's persistence layer (load/save), proving that behavior is preserved after refactoring.

## Files

### Create
- `internal/store/store_persistence_test.go` — Characterization tests for Store's current load/save behavior (validates file persistence, error handling, and field preservation)

## Types and Signatures

```go
// No new production types. Tests exercise existing store behavior:
// store.New(path string) *Store
// (*Store).Add(title string, description string, priority model.Priority) (*model.Task, error)
// (*Store).List() ([]model.Task, error)
// (*Store).Get(id int) (*model.Task, error)
// (*Store).Complete(id int) error
// (*Store).Delete(id int) error
// (*Store).Update(id int, title *string, description *string, priority *model.Priority, status *model.Status) error
// (*Store).Search(query string) ([]model.Task, error)
// (*Store).Count() (int, error)
// (*Store).CountByStatus(status model.Status) (int, error)
// (*Store).ListByStatus(status model.Status) ([]model.Task, error)
// (*Store).ListByPriority(priority model.Priority) ([]model.Task, error)

// Test helper pattern (matching existing store_test.go conventions):
// dir := t.TempDir()
// s := store.New(filepath.Join(dir, "tasks.json"))
```

## Error Types

None — test-only phase.

## Dependencies

None.

## DO NOT

- [ ] Do NOT modify any production code — only add test files
- [ ] Do NOT modify existing test files
- [ ] Do NOT import external test libraries (use standard `testing` package)

## Test Cases

### TestStoreLoadSaveRoundTrip
**Setup:** Create temp directory via `dir := t.TempDir()`, path via `path := filepath.Join(dir, "tasks.json")`, create Store via `s := store.New(path)`
**Input:** Add 3 tasks via:
1. `s.Add("Task 1", "Description 1", model.PriorityHigh)`
2. `s.Add("Task 2", "Description 2", model.PriorityMedium)`
3. `s.Add("Task 3", "Description 3", model.PriorityLow)`, then `s.Complete(2)` to set task 2's status to completed. Create a NEW Store `s2 := store.New(path)` and call `s2.List()`
**Expected:** The new store returns 3 tasks. Verify by comparing Title, Description, Priority, and Status fields for each task against the values used in Add/Complete calls above.

### TestStoreEmptyOrMissingFile
**Setup:** Create Store pointing to a non-existent file path: `s := store.New(filepath.Join(t.TempDir(), "nonexistent.json"))`
**Input:** Call `tasks, err := s.List()`
**Expected:** `err == nil` and `len(tasks) == 0` (store handles missing file gracefully). Then create a second test case within the same test function: write empty file via `emptyPath := filepath.Join(t.TempDir(), "empty.json"); os.WriteFile(emptyPath, []byte{}, 0644)`, create Store `s2 := store.New(emptyPath)`, call `tasks2, err2 := s2.List()`, verify `err2 == nil` and `len(tasks2) == 0`.

### TestStoreCorruptFile
**Setup:** Write `{invalid json` (literal string) to a temp file via `os.WriteFile(path, []byte("{invalid json"), 0644)`, create Store via `s := store.New(path)`
**Input:** Call `s.List()`
**Expected:** Returns nil slice and non-nil error (any non-nil error confirms corrupt file is detected - do not assert on specific error message text).

### TestStoreIDCounterAfterReload
**Setup:** Create Store, add tasks with IDs 1, 2, 3 via three `s.Add()` calls
**Input:** Create new Store pointing to same file, call `Add("New task", "", model.PriorityLow)`, get the returned task
**Expected:** New task has ID 4 (proves nextID counter is reconstructed correctly from file)

### TestStoreIDCounterWithGaps
**Setup:** Create Store, add three tasks (IDs 1,2,3), delete task 2, reload store via `s2 := store.New(path)`
**Input:** Call `s2.Add("After gap", "", model.PriorityLow)`
**Expected:** New task has ID 4, not ID 2 (proves deleted IDs are not reused, counter continues from max)

### TestStoreDeletePersists
**Setup:** Create Store, add two tasks, delete the first task via `s.Delete(1)`, reload store via `s2 := store.New(path)`
**Input:** Call `s2.List()`
**Expected:** Returns slice with one task (ID 2 only), deleted task not present

### TestStoreUpdatePersists
**Setup:** Create Store, add task, update it via `newTitle := "Updated"; s.Update(1, &newTitle, nil, nil, nil)`, reload store via `s2 := store.New(path)`
**Input:** Call `s2.Get(1)`
**Expected:** Task title is "Updated", other fields unchanged from original

### TestStoreInvalidIDOperations
**Setup:** Create Store (empty)
**Input:** Call `s.Get(999)`, `s.Complete(999)`, and `s.Delete(999)`
**Expected:** All three operations return non-nil errors

### TestStoreSearchBehavior
**Setup:** Create Store via `s := store.New(filepath.Join(t.TempDir(), "tasks.json"))`, add tasks via `s.Add("Buy milk", "", model.PriorityLow)`, `s.Add("Buy bread", "", model.PriorityLow)`, `s.Add("Buy CHEESE", "", model.PriorityLow)`, `s.Add("Sell car", "", model.PriorityLow)`
**Input:** Call `s.Search("nothing")`, `s.Search("Buy")`, `s.Search("cheese")`, `s.Search("CHE")`
**Expected:** 
- `s.Search("nothing")` returns empty slice `[]model.Task{}`
- `s.Search("Buy")` returns slice with length 3 containing tasks with titles "Buy milk", "Buy bread", "Buy CHEESE" (order not specified)
- `s.Search("cheese")` returns slice with length 1 containing task with title "Buy CHEESE" (proves case-insensitive matching if this passes; if it returns empty slice, search is case-sensitive)
- `s.Search("CHE")` returns slice with length 1 containing task with title "Buy CHEESE" (proves substring matching works with case-insensitive search; if it returns empty slice, confirms case-sensitivity)

### TestStoreCountOperations
**Setup:** Create Store, add 3 tasks, complete task 1 via `s.Complete(1)`
**Input:** Call `s.Count()`, `s.CountByStatus(model.StatusCompleted)`, `s.CountByStatus(model.StatusPending)`
**Expected:** Count returns 3, CountByStatus(Completed) returns 1, CountByStatus(Pending) returns 2

### TestStoreListByFilters
**Setup:** Create Store, add tasks "Task 1" (High), "Task 2" (Medium), "Task 3" (Low), complete task 2 via `s.Complete(2)`
**Input:** Call `s.ListByStatus(model.StatusCompleted)` and `s.ListByPriority(model.PriorityHigh)`
**Expected:** ListByStatus returns 1 task (ID 2, "Task 2"), ListByPriority returns 1 task (ID 1, "Task 1")

### TestStoreConcurrentAccess
**Setup:** Create Store `s1` via `dir := t.TempDir(); path := filepath.Join(dir, "tasks.json"); s1 := store.New(path)`, add task via `s1.Add("First task", "", model.PriorityLow)`, create second Store `s2 := store.New(path)`
**Input:** Call `s2.Add("Second store task", "", model.PriorityMedium)`, then call `tasks := s1.List()`
**Expected:** `len(tasks) == 2` and tasks contain both "First task" and "Second store task" in their Title fields (proves each method call re-reads file from disk rather than caching)

### TestStorePreservesAllFields
**Setup:** Capture `testStart := time.Now()` at the very beginning of the test. Create Store via `dir := t.TempDir(); path := filepath.Join(dir, "tasks.json"); s := store.New(path)`. Call `s.Add("Test task", "A description", model.PriorityHigh)`, then `s.Complete(1)`.
**Input:** Create new Store `s2 := store.New(path)`, call `task, err := s2.Get(1)`, verify `err == nil`
**Expected:** All fields preserved exactly:
- `task.Title == "Test task"`
- `task.Description == "A description"`
- `task.Status == model.StatusCompleted`
- `task.Priority == model.PriorityHigh`
- `task.CreatedAt.IsZero() == false`, and `task.CreatedAt.Sub(testStart) >= 0` and `task.CreatedAt.Sub(testStart) <= 5*time.Second`
- `task.CompletedAt != nil`, and `task.CompletedAt.Sub(testStart) >= 0` and `task.CompletedAt.Sub(testStart) <= 5*time.Second`

## Edge Cases

1. **Empty store file (0 bytes)** — Should return empty list `[]model.Task{}`, not error (covered by TestStoreEmptyOrMissingFile)
2. **File does not exist** — Should return empty list, store creates it on first save (covered by TestStoreEmptyOrMissingFile)
3. **All task fields survive persistence** — Every field in model.Task round-trips through JSON correctly (covered by TestStorePreservesAllFields)
4. **ID counter persistence** — After reload, new tasks get IDs continuing from the max existing ID, not restarting from 1 (covered by TestStoreIDCounterAfterReload)
5. **ID counter with gaps** — Deleted task IDs are not reused; counter continues from max ID seen (covered by TestStoreIDCounterWithGaps)
6. **Delete operations persist** — Deleted tasks do not reappear after reload (covered by TestStoreDeletePersists)
7. **Update operations persist** — Updated fields survive reload (covered by TestStoreUpdatePersists)
8. **Operations on non-existent IDs fail** — Get/Complete/Delete return errors for invalid IDs (covered by TestStoreInvalidIDOperations)
9. **File re-read on every operation** — Changes made by one Store instance are immediately visible to another instance pointing to the same file (covered by TestStoreConcurrentAccess)

## Integration Points

### Consumed by
- Phase refactor: These tests serve as the safety net. They must all pass after the refactor.

### Depends on
- Nothing — first phase

### Exports
- Characterization test suite that validates persistence behavior
