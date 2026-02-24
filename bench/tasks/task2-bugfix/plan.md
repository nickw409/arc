# Task 2: Implementation Plan — Fix Task Corruption Bug

## Phase 1: Investigation
1. Run the failing test to confirm the bug
2. Read `internal/store/store.go`, specifically the `Complete` method
3. Trace the logic: load -> find task -> modify -> save
4. Identify the off-by-one or aliasing issue

## Phase 2: Root Cause Analysis
1. The bug is in the `Complete` method's attempt to modify task data
2. Look for any code that writes to indices other than the target task
3. Check if any sorting or reindexing happens that could shift data
4. Verify the save path doesn't reorder or corrupt data

## Phase 3: Fix
1. Modify `Complete` to only change fields on the target task (status, completed_at)
2. Remove any code that modifies adjacent tasks
3. Ensure completed tasks retain their original priority

## Phase 4: Verification
1. Run the failing test — it should now pass
2. Run all existing tests — no regressions
3. Add an additional test: complete the first task, last task, and a middle task to verify no edge case corruption
