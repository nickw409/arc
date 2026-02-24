# Phase: investigate

## Objective

Reproduce the task corruption bug, identify the root cause in `store.Complete()`, and write regression tests that prove the bug exists and define correct behavior.

## Files

### Create
- `internal/store/complete_regression_test.go` — Regression tests for the Complete corruption bug

### Modify
- None — this phase only investigates and writes tests, no implementation changes

## Types and Signatures

```go
// No new types. Tests use existing types:
// store.New(path string) *Store
// (*Store).Add(title string, description string, priority model.Priority) (*model.Task, error)
// (*Store).Complete(id int) error
// (*Store).Get(id int) (*model.Task, error)

// Priority constant values (from model package):
// model.PriorityLow = 1
// model.PriorityMedium = 2
// model.PriorityHigh = 3

// Test file package declaration:
// package store_test

// Required imports:
// import (
//   "path/filepath"
//   "testing"
//   "tkit/internal/model"
//   "tkit/internal/store"
// )

// Test helper pattern for temp store creation:
// dir := t.TempDir()
// s := store.New(filepath.Join(dir, "tasks.json"))

// Assertion pattern for all tests:
// Use standard Go testing with t.Fatalf for nil checks, t.Errorf for value mismatches
// Example: if err != nil { t.Fatalf("expected no error, got %v", err) }
// Example: if task.Priority != model.PriorityHigh { t.Errorf("expected Priority=%d, got %d", model.PriorityHigh, task.Priority) }
```

## Error Types

None — test-only phase.

## Dependencies

None.

## DO NOT

- [ ] Do NOT modify `internal/store/store.go` — the fix is in the next phase
- [ ] Do NOT modify any source code — only add test files
- [ ] Do NOT modify existing test files

## Test Cases

### TestCompleteDoesNotCorruptNextTask
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 3 tasks in order:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1 (slice index 0), Priority=1
- `s.Add("Task B", "", model.PriorityHigh)` → ID 2 (slice index 1), Priority=3
- `s.Add("Task C", "", model.PriorityMedium)` → ID 3 (slice index 2), Priority=2
**Input:** Call `s.Complete(2)` on the middle task (ID 2, priority High)
**Expected:**
- `err := s.Complete(2)` must return nil. Check with `if err != nil { t.Fatalf("Complete(2) failed: %v", err) }`
- `task, err := s.Get(2)` must return nil error and task with `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityHigh`. Check all three values. The completed task's own priority must remain unchanged.
- `task, err := s.Get(3)` must return nil error and task with `task.Priority == model.PriorityMedium`. Check both. This verifies the bug is present (priority corruption).
- `task, err := s.Get(1)` must return nil error and task with `task.Priority == model.PriorityLow`. Check both.

### TestCompleteLastTaskNoCrash
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 2 tasks:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1 (slice index 0)
- `s.Add("Task B", "", model.PriorityHigh)` → ID 2 (slice index 1)
**Input:** Call `s.Complete(2)` on the last task
**Expected:**
- `err := s.Complete(2)` must return nil. Check with `if err != nil { t.Fatalf("Complete(2) failed: %v", err) }`
- No index-out-of-bounds panic (test must not crash)
- `task, err := s.Get(1)` must return nil error and task with `task.Priority == model.PriorityLow`. Check both.

### TestCompleteFirstTask
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 3 tasks:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1 (slice index 0), Priority=1
- `s.Add("Task B", "", model.PriorityHigh)` → ID 2 (slice index 1), Priority=3
- `s.Add("Task C", "", model.PriorityMedium)` → ID 3 (slice index 2), Priority=2
**Input:** Call `s.Complete(1)` on the first task
**Expected:**
- `err := s.Complete(1)` must return nil. Check with `if err != nil { t.Fatalf("Complete(1) failed: %v", err) }`
- `task, err := s.Get(1)` must return nil error and task with `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityLow`. Check all three values. The completed task's own priority must remain unchanged.
- `task, err := s.Get(2)` must return nil error and task with `task.Priority == model.PriorityHigh`. Check both. This verifies the bug is present (priority corruption on next task).
- `task, err := s.Get(3)` must return nil error and task with `task.Priority == model.PriorityMedium`. Check both.

### TestCompletePreservesPriority
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add one task:
- `s.Add("Solo task", "", model.PriorityHigh)` → ID 1, Priority=3
**Input:** Call `s.Complete(1)`
**Expected:**
- `err := s.Complete(1)` must return nil. Check with `if err != nil { t.Fatalf("Complete(1) failed: %v", err) }`
- `task, err := s.Get(1)` must return nil error and task with `task.Priority == model.PriorityHigh` and `task.Status == model.StatusCompleted`. Check all three values. The priority should NOT be zeroed on the completed task itself.

### TestCompleteAlreadyCompletedTask
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 3 tasks:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1, Priority=1
- `s.Add("Task B", "", model.PriorityHigh)` → ID 2, Priority=3
- `s.Add("Task C", "", model.PriorityMedium)` → ID 3, Priority=2
Complete task 2 first: `s.Complete(2)`
**Input:** Call `s.Complete(2)` again on the already-completed task
**Expected:**
- `err := s.Complete(2)` (second call) must return nil. Check with `if err != nil { t.Fatalf("Complete(2) second call failed: %v", err) }`
- `task, err := s.Get(2)` must return nil error and task with `task.Status == model.StatusCompleted` and `task.Priority == model.PriorityHigh`. Check all three values. Re-completion must not corrupt the task itself.
- `task, err := s.Get(1)` must return nil error and task with `task.Priority == model.PriorityLow`. Check both.
- `task, err := s.Get(3)` must return nil error and task with `task.Priority == model.PriorityMedium`. Check both. This verifies no corruption on re-completion.

### TestCompleteInvalidID
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 2 tasks:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1
- `s.Add("Task B", "", model.PriorityHigh)` → ID 2
**Input:** Call `s.Complete(999)`
**Expected:**
- `err := s.Complete(999)` must return non-nil error. Check with `if err == nil { t.Fatalf("expected error for invalid ID, got nil") }`
- `task, err := s.Get(1)` must return nil error. Check with `if err != nil { t.Fatalf("Get(1) failed: %v", err) }`. Task must have `task.Priority == model.PriorityLow`. Check with `if task.Priority != model.PriorityLow { t.Errorf("expected Priority=%d, got %d", model.PriorityLow, task.Priority) }`
- `task, err := s.Get(2)` must return nil error. Check with `if err != nil { t.Fatalf("Get(2) failed: %v", err) }`. Task must have `task.Priority == model.PriorityHigh`. Check with `if task.Priority != model.PriorityHigh { t.Errorf("expected Priority=%d, got %d", model.PriorityHigh, task.Priority) }`

### TestCompleteEmptyStore
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Do NOT add any tasks.
**Input:** Call `s.Complete(1)`
**Expected:**
- `err := s.Complete(1)` must return non-nil error. Check with `if err == nil { t.Fatalf("expected error for empty store, got nil") }`
- No panic or index-out-of-bounds error (test must complete without crash)

### TestCompleteZeroID
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 1 task:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1
**Input:** Call `s.Complete(0)`
**Expected:**
- `err := s.Complete(0)` must return non-nil error. Check with `if err == nil { t.Fatalf("expected error for zero ID, got nil") }`
- `task, err := s.Get(1)` must return nil error. Check with `if err != nil { t.Fatalf("Get(1) failed: %v", err) }`. Task must have `task.Priority == model.PriorityLow`. Check with `if task.Priority != model.PriorityLow { t.Errorf("expected Priority=%d, got %d", model.PriorityLow, task.Priority) }`

### TestCompleteNegativeID
**Setup:** Create temp file store via `dir := t.TempDir(); s := store.New(filepath.Join(dir, "tasks.json"))`. Add 1 task:
- `s.Add("Task A", "", model.PriorityLow)` → ID 1
**Input:** Call `s.Complete(-5)`
**Expected:**
- `err := s.Complete(-5)` must return non-nil error. Check with `if err == nil { t.Fatalf("expected error for negative ID, got nil") }`
- `task, err := s.Get(1)` must return nil error. Check with `if err != nil { t.Fatalf("Get(1) failed: %v", err) }`. Task must have `task.Priority == model.PriorityLow`. Check with `if task.Priority != model.PriorityLow { t.Errorf("expected Priority=%d, got %d", model.PriorityLow, task.Priority) }`

## Edge Cases

1. **Complete the last task in the list** — No task after it to corrupt, should not panic (covered by TestCompleteLastTaskNoCrash)
2. **Complete the only task** — Single-element list edge case (covered by TestCompletePreservesPriority)
3. **Complete already-completed task** — Should still not corrupt neighbors (covered by TestCompleteAlreadyCompletedTask)
4. **Invalid task ID** — `Complete(999)` returns error without corrupting any tasks (covered by TestCompleteInvalidID)

## Integration Points

### Consumed by
- Phase fix: These tests define the acceptance criteria for the bug fix. They should FAIL before the fix and PASS after.

### Depends on
- Nothing — first phase

### Exports
- Regression test file used to validate the fix phase
