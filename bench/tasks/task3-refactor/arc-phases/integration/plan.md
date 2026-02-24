# Phase: integration

## Objective

Write integration tests that verify the refactored Store with Backend interface works correctly end-to-end, testing backend swapability, backward compatibility, and mutation isolation.

## Files

### Create
- `internal/store/refactor_integration_test.go` — Integration tests for the Backend refactor

## Types and Signatures

```go
// No new production types. Tests use types from the refactor phase:
// store.Backend (interface with Load/Save)
// store.NewJSONBackend(path string) *JSONBackend
// store.NewMemoryBackend(initial ...[]model.Task) *MemoryBackend
// store.New(path string) *Store
// store.NewWithBackend(b Backend) *Store

// Store method signatures (from existing code):
// Add(title, description string, priority model.Priority) (*model.Task, error)
// List() ([]model.Task, error)
// Get(id int) (*model.Task, error)
// Delete(id int) error
// Update(id int, title, description *string, priority *model.Priority, status *model.Status) error
// Complete(id int) error
// Search(query string) ([]model.Task, error)
// Count() (int, error)
// CountByStatus(status model.Status) (int, error)
// ListByStatus(status model.Status) ([]model.Task, error)
// ListByPriority(priority model.Priority) ([]model.Task, error)

// Test helper pattern:
// dir := t.TempDir()
// path := filepath.Join(dir, "tasks.json")

// Pointer helpers — define at package level in refactor_integration_test.go before any test functions:
func strPtr(s string) *string { return &s }
func statusPtr(s model.Status) *model.Status { return &s }
func priorityPtr(p model.Priority) *model.Priority { return &p }

// Error handling pattern for all tests:
if err != nil {
    t.Fatalf("operation failed: %v", err)
}

// Task comparison pattern: compare core fields (ID, Title, Description, Priority, Status)
// Do not compare timestamps (CreatedAt, CompletedAt) as they vary across backends
// Use this helper defined at package level:
func tasksEqual(t *testing.T, expected, actual []model.Task) {
    if len(expected) != len(actual) {
        t.Fatalf("length mismatch: expected %d tasks, got %d", len(expected), len(actual))
    }
    for i := range expected {
        if expected[i].ID != actual[i].ID ||
           expected[i].Title != actual[i].Title ||
           expected[i].Description != actual[i].Description ||
           expected[i].Priority != actual[i].Priority ||
           expected[i].Status != actual[i].Status {
            t.Errorf("task %d mismatch:\nexpected: %+v\ngot: %+v", i, expected[i], actual[i])
        }
    }
}
```

## Error Types

None — test-only phase.

## Dependencies

**Standard Library:**
- `testing` — test framework
- `path/filepath` — for `filepath.Join` in test setup

**Project Packages:**
- `internal/model` — for `model.Task`, `model.Status*`, `model.Priority*` constants
- `internal/store` — for `store.Backend`, `store.New`, `store.NewWithBackend`, `store.NewJSONBackend`, `store.NewMemoryBackend`

**Pre-Execution Validation:**
Before writing tests, verify the refactor phase completed successfully:
1. Check that `internal/store/backend.go` exists and exports `Backend` interface
2. Check that `internal/store/store.go` exports `NewJSONBackend`, `NewMemoryBackend`, and `NewWithBackend`
3. If any of these are missing, STOP and report: "Refactor phase incomplete — cannot proceed with integration tests"

## DO NOT

- [ ] Do NOT modify any production code — only add test files
- [ ] Do NOT modify existing test files
- [ ] Do NOT duplicate backend unit tests from the refactor phase (TestJSONBackendRoundTrip, TestMemoryBackendRoundTrip, TestMemoryBackendIsolation, etc.)
- [ ] Do NOT modify `internal/store/backend.go` or `internal/store/store.go`

## Test Cases

### TestIntegrationBackendSwap
**Setup:** Create a sequence of operations: Add 3 tasks with titles "Task 1", "Task 2", "Task 3" (all with empty descriptions and PriorityLow), Complete task ID 2, Delete task ID 1, Update task ID 3 title to "Updated".
**Input:** Run the exact same operation sequence against both:
1. JSON-backed store: `dir := t.TempDir(); path := filepath.Join(dir, "tasks.json"); s1 := store.NewWithBackend(store.NewJSONBackend(path))`
2. Memory-backed store: `s2 := store.NewWithBackend(store.NewMemoryBackend())`
For each store:
- Call `_, err := s.Add("Task 1", "", model.PriorityLow)` → check err is nil
- Call `_, err = s.Add("Task 2", "", model.PriorityLow)` → check err is nil
- Call `_, err = s.Add("Task 3", "", model.PriorityLow)` → check err is nil
- Call `s.Complete(2)` → check error is nil
- Call `s.Delete(1)` → check error is nil
- Call `s.Update(3, strPtr("Updated"), nil, nil, nil)` → check error is nil
- Call `tasks, err := s.List()` → check error is nil
**Expected:** Both List() results return exactly 2 tasks. Before comparison, sort both slices by ID using `sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })`. Then use the tasksEqual helper to verify both stores return identical tasks (comparing ID, Title, Description, Priority, Status fields). Task with ID 2 must have Status == model.StatusCompleted. Task with ID 3 must have Title == "Updated".

### TestIntegrationNewBackwardCompat
**Setup:** Create store via `store.New(path)` (the original constructor)
**Input:** Run a full lifecycle:
1. `s.Add("Task 1", "desc 1", model.PriorityLow)` → returns task with ID 1
2. `s.Add("Task 2", "desc 2", model.PriorityHigh)` → returns task with ID 2
3. `s.Get(1)` → returns task with Title "Task 1"
4. `s.Complete(2)` → returns nil
5. `s.List()` → returns 2 tasks
6. `s.Count()` → returns 2
7. `s.CountByStatus(model.StatusCompleted)` → returns 1
8. `s.Search("Task")` → returns 2 tasks
9. `s.ListByStatus(model.StatusPending)` → returns 1 task (Task 1)
10. `s.ListByPriority(model.PriorityHigh)` → returns 1 task (Task 2)
11. `s.Delete(1)` → returns nil
12. `s.List()` → returns 1 task
**Expected:** Every operation works identically to before the refactor

### TestIntegrationMemoryBackendIsolation
**Setup:** Create `s := store.NewWithBackend(store.NewMemoryBackend())`, add 2 tasks: `s.Add("Original Title 1", "desc1", model.PriorityLow)` and `s.Add("Original Title 2", "desc2", model.PriorityHigh)`. Check both Add calls return no error.
**Input:**
1. Call `tasks1, err := s.List()` → check error is nil
2. Store original title: `originalTitle := tasks1[0].Title` (should be "Original Title 1")
3. Mutate the returned slice: `tasks1[0].Title = "MUTATED"`
4. Verify mutation took effect in local variable: check `tasks1[0].Title == "MUTATED"`
5. Call `tasks2, err := s.List()` → check error is nil
**Expected:** `tasks2[0].Title == originalTitle` (i.e., "Original Title 1", NOT "MUTATED"). This proves mutation isolation: the returned slice is a deep copy, not a reference to internal state.

### TestIntegrationAllExistingOps
**Setup:** Create `s := store.NewWithBackend(store.NewMemoryBackend())`
**Input:** Execute every public Store method in sequence:
1. `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1, no error
2. `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2, no error
3. `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3, no error
4. `s.Get(1)` → returns task with Title "Task A", no error
5. `s.List()` → returns 3 tasks, no error
6. `s.Update(1, strPtr("Updated A"), nil, nil, nil)` → no error
7. `s.Update(2, nil, strPtr("Updated desc"), nil, nil)` → no error (description only)
8. `s.Update(3, nil, nil, &model.StatusCompleted, nil)` → no error (status only)
9. `s.Update(1, strPtr("Final A"), strPtr("Final desc"), &model.StatusCompleted, &model.PriorityHigh)` → no error (all fields)
10. `s.Complete(2)` → no error
11. `s.Delete(3)` → no error
12. `s.Search("Updated")` → returns 1 task (Task A), no error
13. `s.Search("Final")` → returns 1 task (Task A), no error
14. `s.Count()` → returns 2, no error
15. `s.CountByStatus(model.StatusCompleted)` → returns 2, no error
16. `s.ListByStatus(model.StatusPending)` → returns 0 tasks, no error
17. `s.ListByPriority(model.PriorityHigh)` → returns 2 tasks, no error
**Expected:** All operations succeed with correct results when backed by MemoryBackend, including Update with various nil combinations

## Edge Cases

1. **JSON file persistence across store instances** — Create Store A via New(path), add tasks, create Store B via New(same path), verify B sees A's tasks
2. **MemoryBackend does not persist** — Two separate NewWithBackend(NewMemoryBackend()) stores do NOT share state
3. **Empty store operations** — `List()`, `Search("anything")`, `Count()`, `ListByStatus(...)`, `ListByPriority(...)` on empty MemoryBackend all return empty slices/zero, not nil or errors
4. **Error paths** — `Get(999)`, `Delete(999)`, `Update(999, ...)`, `Complete(999)` on nonexistent IDs all return errors (not panics)
5. **Search with empty string** — `Search("")` returns all tasks (matches everything)
6. **Search with no matches** — `Search("NONEXISTENT")` returns empty slice, not error
7. **ID generation after deletion** — Add 3 tasks (IDs 1,2,3), delete task 2, add another task → new task gets ID 4 (not ID 2)

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase characterize: Characterization tests in `internal/store/backend_test.go` validate that the original behavior is preserved
- Phase refactor: `store.Backend` interface, `store.JSONBackend`, `store.MemoryBackend`, `store.NewWithBackend`, and refactored `store.New` must all exist

**CRITICAL:** If you cannot import or reference the types listed above (Backend, JSONBackend, MemoryBackend, NewWithBackend), the refactor phase is incomplete. Do NOT proceed. Report the missing types and exit.

### Exports
- None — this is the terminal phase
