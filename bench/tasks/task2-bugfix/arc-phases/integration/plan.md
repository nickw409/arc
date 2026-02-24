# Phase: integration

## Objective

Write integration tests that exercise the Complete workflow end-to-end, verifying that the bug fix holds under realistic multi-task scenarios with interleaved operations.

## Files

### Create
- `internal/store/complete_integration_test.go` — Integration tests for Complete correctness across workflows

## Types and Signatures

```go
// Package declaration: package store (same package as store.go, white-box)
//
// Imports:
//   "path/filepath"
//   "testing"
//   "github.com/nwiley/tkit/internal/model"
//
// No new production types. Tests use existing types:
// store.New(path string) *Store
// (*Store).Add(title string, description string, priority model.Priority) (*model.Task, error)
// (*Store).Complete(id int) error
// (*Store).Get(id int) (*model.Task, error)
// (*Store).List() ([]model.Task, error)
//
// Test setup pattern (repeat in each test, do NOT extract a helper):
//   dir := t.TempDir()
//   s := store.New(filepath.Join(dir, "tasks.json"))
//
// Assertion conventions:
// - Compare Priority using constants: task.Priority == model.PriorityMedium
//   (NOT numeric values like int(task.Priority) == 2)
// - Compare Status using constants: task.Status == model.StatusCompleted
// - For Get() results: task, err := s.Get(id); if err != nil { t.Fatalf("...") }
// - For List() results: Iterate the slice with a loop, match tasks by task.ID field.
//   If a required ID is not found in the slice, call t.Fatalf.
// - All assertion failures must use t.Errorf or t.Fatalf (prefer t.Fatalf for setup failures,
//   t.Errorf for condition checks so multiple assertions can be reported in one run).
// - model.PriorityNone is the zero value of the Priority type — checking that no task
//   has Priority == model.PriorityNone verifies that priorities were not zeroed out.
```

## Error Types

None — test-only phase.

## Dependencies

None new — uses standard `testing` package and existing project packages.

## DO NOT

- [ ] Do NOT modify any production code — only add test files
- [ ] Do NOT modify existing test files
- [ ] Do NOT duplicate regression tests from the investigate phase (TestCompleteDoesNotCorruptNextTask, TestCompleteLastTaskNoCrash, TestCompleteMiddleTask, TestCompletePreservesPriority, TestCompleteAlreadyCompletedTask)
- [ ] Do NOT modify `internal/store/complete_regression_test.go`

## Test Cases

### TestIntegrationCompleteWorkflow
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 5 tasks:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3
- `s.Add("Task D", "desc D", model.PriorityHigh)` → ID 4
- `s.Add("Task E", "desc E", model.PriorityLow)` → ID 5
**Input:** Call `s.Complete(3)` (middle task)
**Expected:**
- `Get(3)` returns task with `Status == model.StatusCompleted` and `CompletedAt != nil`
- `Get(3)` returns task with `Priority == model.PriorityMedium` (retained)
- `Get(1)` returns task with `Priority == model.PriorityLow` (unchanged)
- `Get(2)` returns task with `Priority == model.PriorityHigh` (unchanged)
- `Get(4)` returns task with `Priority == model.PriorityHigh` (unchanged — this is the adjacent task)
- `Get(5)` returns task with `Priority == model.PriorityLow` (unchanged)

### TestIntegrationCompleteMultiple
**Setup:** Create temp file store, add 5 tasks:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1, Priority=Low
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2, Priority=High
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3, Priority=Medium
- `s.Add("Task D", "desc D", model.PriorityHigh)` → ID 4, Priority=High
- `s.Add("Task E", "desc E", model.PriorityLow)` → ID 5, Priority=Low
**Input:** Complete tasks in sequence: `s.Complete(2)`, then `s.Complete(4)`, then `s.Complete(1)`
**Expected:**
- Completed tasks retain their status and priority:
  - `Get(1)`: `Status == model.StatusCompleted`, `Priority == model.PriorityLow`
  - `Get(2)`: `Status == model.StatusCompleted`, `Priority == model.PriorityHigh`
  - `Get(4)`: `Status == model.StatusCompleted`, `Priority == model.PriorityHigh`
- Non-completed tasks are untouched:
  - `Get(3)`: `Status == model.StatusPending`, `Priority == model.PriorityMedium`
  - `Get(5)`: `Status == model.StatusPending`, `Priority == model.PriorityLow`
- Verify via `s.List()`: `err == nil`, `len(tasks) == 5`, iterate all tasks and confirm:
  - No task has `Priority == model.PriorityNone`
  - Exactly 3 tasks have `Status == model.StatusCompleted` (IDs 1, 2, 4)
  - Exactly 2 tasks have `Status == model.StatusPending` (IDs 3, 5)
  - Find each task by ID and verify specific completion state matches expectations above

### TestIntegrationCompleteThenList
**Setup:** Create temp file store, add 3 tasks:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1, Priority=Low
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2, Priority=High
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3, Priority=Medium
**Input:** Call `s.Complete(2)`, then call `tasks, err := s.List()`
**Expected:**
- If `err != nil`, call t.Fatalf. Check `len(tasks) == 3`.
- Iterate the tasks slice. For each task, check task.ID and verify the corresponding fields:
  - If task.ID == 2: `task.Status == model.StatusCompleted`, `task.Priority == model.PriorityHigh`
  - If task.ID == 1: `task.Status == model.StatusPending`, `task.Priority == model.PriorityLow`
  - If task.ID == 3: `task.Status == model.StatusPending`, `task.Priority == model.PriorityMedium`
- After iteration, ensure all three IDs (1, 2, 3) were found. If any ID is missing, call t.Fatalf.

## Edge Cases

1. **Completing tasks in reverse order** — Complete ID 5, then 4, then 3, then 2, then 1 — no corruption at any step
2. **Completing all tasks** — Every task ends up Completed with its original priority retained
3. **List after multiple completions** — List must reflect all completions without any priority corruption
4. **Interleaved Add and Complete operations** — Add task → complete → add another → complete — verifies state consistency under realistic workflows
5. **File persistence across multiple reads** — Multiple Get() calls on same completed task return consistent results

### TestIntegrationCompleteReverseOrder
**Setup:** Create temp file store, add 5 tasks:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3
- `s.Add("Task D", "desc D", model.PriorityHigh)` → ID 4
- `s.Add("Task E", "desc E", model.PriorityLow)` → ID 5
**Input:** Complete in reverse order: `s.Complete(5)`, `s.Complete(4)`, `s.Complete(3)`, `s.Complete(2)`, `s.Complete(1)`
**Expected:**
- After `s.Complete(5)`, call `task, err := s.Get(5)`: if `err != nil` call t.Fatalf, then verify `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityLow`.
- After `s.Complete(4)`, call `task, err := s.Get(4)`: if `err != nil` call t.Fatalf, then verify `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityHigh`.
- After `s.Complete(3)`, call `task, err := s.Get(3)`: if `err != nil` call t.Fatalf, then verify `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityMedium`.
- After `s.Complete(2)`, call `task, err := s.Get(2)`: if `err != nil` call t.Fatalf, then verify `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityHigh`.
- After `s.Complete(1)`, call `task, err := s.Get(1)`: if `err != nil` call t.Fatalf, then verify `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityLow`.
- After all completions, call `tasks, err := s.List()`: if `err != nil` call t.Fatalf. Check `len(tasks) == 5`.
  Build a map[int]model.Task by iterating tasks and indexing by task.ID.
  Then verify for each ID:
  - map[1].Status == model.StatusCompleted, map[1].Priority == model.PriorityLow
  - map[2].Status == model.StatusCompleted, map[2].Priority == model.PriorityHigh
  - map[3].Status == model.StatusCompleted, map[3].Priority == model.PriorityMedium
  - map[4].Status == model.StatusCompleted, map[4].Priority == model.PriorityHigh
  - map[5].Status == model.StatusCompleted, map[5].Priority == model.PriorityLow
  Iterate the map values and count tasks with Priority == model.PriorityNone. Assert count == 0.

### TestIntegrationCompleteAllTasks
**Setup:** Create temp file store, add 3 tasks:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3
**Input:** Complete all tasks: `s.Complete(1)`, `s.Complete(2)`, `s.Complete(3)`
**Expected:**
- After all completions, `tasks, err := s.List()`:
  - `err == nil`, `len(tasks) == 3`
  - All tasks have `Status == model.StatusCompleted`
  - Task ID 1: `Priority == model.PriorityLow`
  - Task ID 2: `Priority == model.PriorityHigh`
  - Task ID 3: `Priority == model.PriorityMedium`
  - No task has `Priority == model.PriorityNone`

### TestIntegrationInterleavedAddComplete
**Setup:** Create temp file store.
**Input:** Perform interleaved operations:
- `s.Add("Task A", "desc A", model.PriorityLow)` → ID 1
- `s.Complete(1)`
- `s.Add("Task B", "desc B", model.PriorityHigh)` → ID 2
- `s.Add("Task C", "desc C", model.PriorityMedium)` → ID 3
- `s.Complete(2)`
- `s.Add("Task D", "desc D", model.PriorityLow)` → ID 4
**Expected:**
- `Get(1)`: `Status == model.StatusCompleted`, `Priority == model.PriorityLow`
- `Get(2)`: `Status == model.StatusCompleted`, `Priority == model.PriorityHigh`
- `Get(3)`: `Status == model.StatusPending`, `Priority == model.PriorityMedium`
- `Get(4)`: `Status == model.StatusPending`, `Priority == model.PriorityLow`
- `tasks, err := s.List()`: `err == nil`, `len(tasks) == 4`
- No task has `Priority == model.PriorityNone`

### TestIntegrationRepeatedGetAfterComplete
**Setup:** Create temp file store, add 2 tasks:
- `s.Add("Task A", "desc A", model.PriorityHigh)` → ID 1
- `s.Add("Task B", "desc B", model.PriorityMedium)` → ID 2
**Input:** Complete ID 1, then call `Get(1)` three times in succession
**Expected:**
- First `Get(1)`: `Status == model.StatusCompleted`, `Priority == model.PriorityHigh`, `CompletedAt != nil`
- Second `Get(1)`: identical results (verifies file persistence)
- Third `Get(1)`: identical results (verifies file persistence)
- `Get(2)`: `Status == model.StatusPending`, `Priority == model.PriorityMedium`

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase investigate: Regression tests in `internal/store/complete_regression_test.go` define the bug and expected behavior
- Phase fix: The `Complete` method in `internal/store/store.go` must be fixed (priority-zeroing block removed) for these tests to pass

### Exports
- None — this is the terminal phase
