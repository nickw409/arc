# Task 2: Fix Task Corruption Bug

## Problem

Users report that completing a task sometimes corrupts data on other tasks. Specifically, after marking a task as complete, adjacent tasks in the task list lose their priority (priority becomes "none"/0).

## Reproduction

A failing test is provided in `failing_test.go`. Copy it into `internal/store/` and run:
```
go test ./internal/store/ -run TestCompleteDoesNotCorruptAdjacentTask
```

## Steps to Reproduce Manually

1. Add three tasks with different priorities:
   ```
   tkit add "Task A" -p low
   tkit add "Task B" -p high
   tkit add "Task C" -p medium
   ```
2. Complete task B (ID 2):
   ```
   tkit complete 2
   ```
3. Show task C (ID 3):
   ```
   tkit show 3
   ```
4. **Expected:** Task C has priority "medium"
5. **Actual:** Task C has priority "none" — its priority was zeroed out

## Requirements

- Fix the root cause of the corruption
- The failing test must pass
- All existing tests must continue to pass
- Completed tasks should retain their original priority (do NOT zero out priority on completion)
- No other task's data should be modified when completing a task
