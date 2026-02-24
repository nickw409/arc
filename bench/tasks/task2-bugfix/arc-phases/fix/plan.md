# Phase: fix

## Objective

Fix the off-by-one bug in `Store.Complete()` that corrupts the adjacent task's priority, and verify all tests pass.

## Files

### Modify
- `internal/store/store.go` — Fix the `Complete` method

## Types and Signatures

```go
// No signature changes. The fix is inside the existing method:
// func (s *Store) Complete(id int) error

// THE BUG: Inside the Complete method, there is code that sets tasks[i+1].Priority = 0
// (the NEXT task's priority) instead of leaving it unchanged. This code must be removed.
//
// THE FIX: In the for-loop where tasks[i].ID == id is found, locate and delete any code that
// modifies tasks[i+1].Priority. This will typically be an if-statement checking `i+1 < len(tasks)` 
// followed by `tasks[i+1].Priority = 0`. Delete the entire if-block. If there are comment lines 
// directly above this if-block (with no blank lines in between), delete those comment lines as well.
// After deletion, run `gofmt -w internal/store/store.go` to ensure proper formatting.
//
// CORRECT BEHAVIOR: Completed tasks retain their original priority value.
//
// After the fix, the Complete method body (inside the for-loop where tasks[i].ID == id)
// must contain exactly these operations:
//   1. tasks[i].Status = model.StatusCompleted
//   2. now := time.Now(); tasks[i].CompletedAt = &now
//   3. return s.save(tasks)
//
// No other modifications to tasks[i] or any other task should occur in this method.
```

**Regression tests** may exist in `internal/store/complete_regression_test.go`. Before making any changes, check if this file exists. If it exists, you will run it as part of verification. If it does not exist, skip any test cases that reference it. The Test Cases defined below are the acceptance criteria regardless of whether the regression test file exists.

## Error Types

No changes.

## Dependencies

None.

## DO NOT

- [ ] Do NOT change the signature of `Complete`
- [ ] Do NOT change any other method in store.go
- [ ] Do NOT add new methods
- [ ] Do NOT add new test files (rely on existing or investigate-phase regression tests)
- [ ] Do NOT change how `Complete` sets the status or completed_at timestamp — only fix the priority corruption

## Test Cases

### TestCompleteMiddleTaskFixed
**Setup:** Create temp file store, add 3 tasks: Task A (PriorityLow=1), Task B (PriorityHigh=3), Task C (PriorityMedium=2)
**Input:** Call `s.Complete(2)`
**Expected:**
- `Get(2)` returns task with `Status == model.StatusCompleted` and `Priority == model.PriorityHigh` (value 3, retained)
- `Get(2)` returns task with `CompletedAt != nil`
- `Get(3)` returns task with `Priority == model.PriorityMedium` (value 2, not corrupted)
- `Get(1)` returns task with `Priority == model.PriorityLow` (value 1, unchanged)

### TestCompleteRetainsPriorityFixed
**Setup:** Create temp file store, add 1 task with PriorityHigh(3)
**Input:** Call `s.Complete(1)`
**Expected:** `Get(1)` returns task with `Priority == model.PriorityHigh` (value 3, completed task retains its own priority)

### TestCompleteLastTaskNoCrash
**Setup:** Create temp file store, add 3 tasks
**Input:** Call `s.Complete(3)` (last task)
**Expected:**
- `Get(3)` returns task with `Status == model.StatusCompleted`
- `Get(3)` returns task with `CompletedAt != nil`
- No panic or index-out-of-bounds error occurs
- `Get(1)` and `Get(2)` are unchanged

### TestCompleteFirstTask
**Setup:** Create temp file store, add 3 tasks with PriorityLow(1), PriorityMedium(2), PriorityHigh(3)
**Input:** Call `s.Complete(1)` (first task)
**Expected:**
- `Get(1)` returns task with `Status == model.StatusCompleted` and `Priority == model.PriorityLow` (value 1, retained)
- `Get(2)` returns task with `Priority == model.PriorityMedium` (value 2, unchanged)

### TestCompleteNonExistentTask
**Setup:** Create temp file store, add 2 tasks
**Input:** Call `s.Complete(999)`
**Expected:**
- Error is returned
- `List()` returns exactly 2 tasks, both unchanged
- Store file content is unchanged

### TestCompleteAlreadyCompletedTask
**Setup:** Create temp file store, add 1 task, complete it (first call to `Complete(1)`), capture the `CompletedAt` timestamp
**Input:** Call `s.Complete(1)` again
**Expected:**
- No error (idempotent operation)
- `Get(1)` returns task with `Status == model.StatusCompleted`
- `CompletedAt` timestamp is unchanged from first completion

### TestCompleteEmptyStore
**Setup:** Create temp file store with no tasks
**Input:** Call `s.Complete(1)`
**Expected:**
- Error is returned
- Store remains empty

### Verify regression tests from investigate phase pass (if they exist)
**Input:** Run `go test ./internal/store/ -run TestComplete -v`
**Expected:** If `complete_regression_test.go` exists, all `TestComplete*` tests should pass, including `TestCompleteDoesNotCorruptNextTask`, `TestCompleteLastTaskNoCrash`, `TestCompleteFirstTask`, `TestCompletePreservesPriority`, `TestCompleteAlreadyCompletedTask`. If the file does not exist, this test case does not apply — rely on the concrete test cases defined earlier in this plan.

### Verify existing tests still pass
**Input:** Run `go test ./...`
**Expected:** All existing tests pass — zero regressions

## Edge Cases

1. **The fix should be minimal** — Only remove the priority-zeroing block (the exact if statement and comment described in Types and Signatures section)
2. **Do not add new behavior** — The Complete method should only set status to Completed and set CompletedAt
3. **Last task boundary** — Completing the last task in the list must not cause index-out-of-bounds panic (the bug involved `tasks[i+1]`)
4. **Non-existent task** — Completing a task ID that doesn't exist must return an error and leave the store unchanged
5. **Already completed task** — Completing an already-completed task must be idempotent (no error, no timestamp change)
6. **Empty store** — Completing any task ID in an empty store must return an error

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase investigate: Regression tests in `complete_regression_test.go` (if they exist) will be used for validation, but the fix can proceed independently

### Exports
- Fixed `Complete` method
