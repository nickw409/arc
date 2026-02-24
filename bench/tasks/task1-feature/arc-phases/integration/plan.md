# Phase: integration

## Objective

Write integration tests that exercise the full label workflow end-to-end, verifying that model, store, filter, and CLI phases work together correctly.

## Files

### Create
- `internal/cli/label_integration_test.go` — Integration tests for the complete label feature (path relative to project root; if project root is `/home/user/tkit`, full path is `/home/user/tkit/internal/cli/label_integration_test.go`)

## Types and Signatures

```go
// Package declaration: package cli (white-box testing, direct access to NewRootCmd)
//
// Imports:
//   "bytes"
//   "path/filepath"
//   "strings"
//   "testing"
//   "github.com/nwiley/tkit/internal/store"  // Exact import path: open the file "go.mod" located in the project root directory, read the first line which has the format "module <module-path>", extract the module path, then append "/internal/store". Example: if go.mod contains "module github.com/nwiley/tkit", use "github.com/nwiley/tkit/internal/store"
//   "github.com/nwiley/tkit/internal/model"  // Same module path prefix (from go.mod first line) + "/internal/model"
//
// No new production types. Tests only.
//
// Testing pattern — this project has these conventions:
//
// 1. The store is initialized via TKIT_FILE env var. Set it per test:
//      dir := t.TempDir()
//      path := filepath.Join(dir, "tasks.json")
//      t.Setenv("TKIT_FILE", path)
//
// 2. Build a root command — NewRootCmd() is accessible directly (same package):
//      cmd := NewRootCmd()  // No import prefix needed — package cli
//
// 3. Set args and capture output:
//      cmd.SetArgs([]string{"add", "task title", "-l", "bug"})
//      var buf bytes.Buffer
//      cmd.SetOut(&buf)
//      cmd.SetErr(&buf)
//
// 4. Execute and assert:
//      err := cmd.Execute()
//      output := buf.String()
//
// 5. For store verification (read state from disk):
//      s := store.New(path)  // same path variable from step 1
//      task, _ := s.Get(1)
//
// IMPORTANT: Create a NEW NewRootCmd() for each command execution within a test,
// because cobra caches state between calls. The TKIT_FILE env var ensures all
// command instances use the same store file.
//
// IMPORTANT: Commands use RunE — errors are returned by cmd.Execute(), not
// printed to stderr via os.Exit. For error tests, check:
//      err != nil                                     // error occurred
//      strings.Contains(err.Error(), "expected msg")  // error message check
// Do NOT check the output buffer for error messages — Cobra puts RunE errors
// in the returned error, not in stdout/stderr buffers.

// Package-level helper function in the test file.
// Location: after the import block, before the first test function.
// Not nested inside any test function.
func runCmd(t *testing.T, args ...string) (string, error) {
    t.Helper()
    cmd := NewRootCmd()
    var buf bytes.Buffer
    cmd.SetOut(&buf)
    cmd.SetErr(&buf)
    cmd.SetArgs(args)
    err := cmd.Execute()
    return buf.String(), err
}
```

## Error Types

None — test-only phase.

## Dependencies

None new — uses standard `testing` package, `bytes`, `strings`, `path/filepath`, and existing project packages (`store`, `model`).

**Hard dependency:** ALL previous phases (model, store, filter, cli) must be complete.

**Pre-execution verification:**
1. Run `go build ./...` from the project root
2. If the build succeeds: proceed with writing tests
3. If the build fails: STOP and report the failure — do NOT attempt to fix compilation errors (they belong to earlier phases)

## DO NOT

- [ ] Do NOT modify any production code — only add test files
- [ ] Do NOT modify existing test files
- [ ] Do NOT duplicate unit tests from model, store, filter, or cli phases — integration tests exercise cross-phase flows only
- [ ] Do NOT test internal implementation details — only test through CLI commands or public store API
- [ ] Do NOT use `os.Exit` in tests — commands return errors via `RunE`

## Test Cases

All test names use Go's `TestXxx` convention.

### TestIntegrationAddTaskWithLabels
**Setup:**
```go
dir := t.TempDir()
path := filepath.Join(dir, "tasks.json")
t.Setenv("TKIT_FILE", path)
```
**Steps:**
1. `_, err := runCmd(t, "add", "Fix login bug", "-l", "bug", "-l", "frontend")` — `err == nil`
2. `out, err := runCmd(t, "show", "1")` — `err == nil`
**Expected:**
- `strings.Contains(out, "Labels:")` is true
- `strings.Contains(out, "bug, frontend")` is true (exact format: comma followed by single space separator, no brackets or quotes)
- The output line must match the pattern `Labels:    bug, frontend` (word "Labels" followed by colon, followed by 4 spaces, followed by comma-separated label list)
- `store.New(path).Get(1)` returns task with `Labels: []string{"bug", "frontend"}`

### TestIntegrationLabelAddRemove
**Setup:** Set TKIT_FILE. Run `runCmd(t, "add", "Test task")` to create task 1.
**Steps:**
1. `out, err := runCmd(t, "label", "add", "1", "urgent")` — err is nil, out contains `Added label "urgent" to task #1`
2. `out, _ := runCmd(t, "show", "1")` — out contains substring `Labels:    urgent` (exact format: word "Labels" followed by colon, followed by exactly 4 spaces, followed by label name)
3. `out, err := runCmd(t, "label", "remove", "1", "urgent")` — err is nil, out contains `Removed label "urgent" from task #1`
4. `out, _ := runCmd(t, "show", "1")` — out does NOT contain substring `Labels:` (when a task has no labels, the Labels field must be completely omitted from show output, not displayed with an empty value)

### TestIntegrationListFilterByLabel
**Setup:** Set TKIT_FILE. Create 3 tasks and add labels:
```go
runCmd(t, "add", "Bug task")         // task 1
runCmd(t, "label", "add", "1", "bug")
runCmd(t, "add", "Feature task")     // task 2
runCmd(t, "label", "add", "2", "feature")
runCmd(t, "add", "Bug+frontend task") // task 3
runCmd(t, "label", "add", "3", "bug")
runCmd(t, "label", "add", "3", "frontend")
```
**Input:** `runCmd(t, "list", "--label", "bug")`
**Expected:**
- err is nil
- Output contains "Bug task" and "Bug+frontend task"
- Output does NOT contain "Feature task"

### TestIntegrationListFilterByMultipleLabels
**Setup:** Same 3 tasks as above
**Input:** `runCmd(t, "list", "--label", "bug", "--label", "frontend")`
**Expected:**
- err is nil
- Output contains only "Bug+frontend task" (AND semantics)
- Output does NOT contain "Bug task" or "Feature task"

### TestIntegrationLabelAcrossMultipleTasks
**Setup:** Set TKIT_FILE. Create 3 tasks:
```go
runCmd(t, "add", "Task 1")
runCmd(t, "label", "add", "1", "shared")
runCmd(t, "add", "Task 2")
runCmd(t, "label", "add", "2", "shared")
runCmd(t, "add", "Task 3")
runCmd(t, "label", "add", "3", "unique")
```
**Steps:**
1. `out, err := runCmd(t, "label", "list")`
2. Assert: `err == nil`
3. Assert: `strings.Count(out, "shared") == 1` (label "shared" appears exactly once in the output, deduplicated across tasks)
4. `listOut, _ := runCmd(t, "list", "--label", "shared")`
5. Assert: `strings.Contains(listOut, "Task 1") && strings.Contains(listOut, "Task 2") && !strings.Contains(listOut, "Task 3")`

### TestIntegrationLabelListAggregation
**Setup:** Set TKIT_FILE. Create 3 tasks with labels:
```go
runCmd(t, "add", "Task 1")
runCmd(t, "label", "add", "1", "bug")
runCmd(t, "add", "Task 2")
runCmd(t, "label", "add", "2", "bug")
runCmd(t, "label", "add", "2", "frontend")
runCmd(t, "add", "Task 3")
runCmd(t, "label", "add", "3", "urgent")
```
**Input:** `out, err := runCmd(t, "label", "list")`
**Expected:** `err == nil`. Output contains "bug", "frontend", and "urgent" on separate lines. "bug" appears exactly once (deduplicated). Labels are sorted alphabetically. No header or footer lines. Implementation note: The CLI phase must output labels as a simple newline-separated list with no decorative text.

### TestIntegrationLabelPersistence
**Setup:** Set TKIT_FILE
**Steps:**
1. `runCmd(t, "add", "Task 1")`
2. `runCmd(t, "label", "add", "1", "persistent")`
3. Create a NEW NewRootCmd() instance to simulate restart: `out, err := runCmd(t, "show", "1")`
**Expected:** `err == nil`. `strings.Contains(out, "persistent")` is true — label survives across command instances

### TestIntegrationLabeledTaskDelete
**Setup:** Set TKIT_FILE. Create 2 tasks with labels:
```go
runCmd(t, "add", "Task 1")
runCmd(t, "label", "add", "1", "ephemeral")
runCmd(t, "add", "Task 2")
runCmd(t, "label", "add", "2", "persistent")
runCmd(t, "label", "add", "2", "bug")
```
**Steps:**
1. `runCmd(t, "delete", "1")`
2. `out, err := runCmd(t, "label", "list")`
**Expected:** `err == nil`. Output contains "bug" and "persistent". Output does NOT contain "ephemeral"

### TestIntegrationManyLabelsPerTask
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Task 1")`.
**Steps:**
1. Add 15 labels: `runCmd(t, "label", "add", "1", "label01")`, `runCmd(t, "label", "add", "1", "label02")`, ... `runCmd(t, "label", "add", "1", "label15")`
2. `out, err := runCmd(t, "show", "1")`
**Expected:** `err == nil`. Output contains all 15 labels in the Labels field

### TestIntegrationLabelErrorNotFound
**Setup:** Set TKIT_FILE (empty store, no tasks)
**Input:** `_, err := runCmd(t, "label", "add", "999", "bug")`
**Expected:** `err != nil && strings.Contains(err.Error(), "not found")` — error returned by `cmd.Execute()` from `RunE`. The error message must contain the substring "not found" (case-sensitive, no other requirements on exact wording).

### TestIntegrationLabelDeduplication
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Test task")`.
**Steps:**
1. `_, err1 := runCmd(t, "label", "add", "1", "bug")` — first add
2. `_, err2 := runCmd(t, "label", "add", "1", "bug")` — second add (same label)
**Expected:** `err1 == nil && err2 == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug"}` (no duplicate)

### TestIntegrationLabelNormalization
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Test task")`.
**Steps (sequential assertions within single test function, NOT separate t.Run() subtests):**
1. `runCmd(t, "label", "add", "1", "  BUG  ")`
2. `out, _ := runCmd(t, "show", "1")`
3. Assert: `strings.Contains(out, "bug")` is true
4. `runCmd(t, "label", "add", "1", "BuGfIx")`
5. `out2, _ := runCmd(t, "show", "1")`
6. Assert: `strings.Contains(out2, "bugfix")` is true

### TestIntegrationEmptyLabelError
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Test task")`.
**Input:** `_, err := runCmd(t, "label", "add", "1", "")`
**Expected:** `err != nil && strings.Contains(err.Error(), "label cannot be empty")` — error message must contain the exact substring "label cannot be empty" (case-sensitive).

### TestIntegrationWhitespaceLabelError
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Test task")`.
**Input:** `_, err := runCmd(t, "label", "add", "1", "   ")`
**Expected:** `err != nil && strings.Contains(err.Error(), "label cannot be empty")` — whitespace-only strings normalize to empty and should be rejected. Error message must contain the exact substring "label cannot be empty" (case-sensitive).

### TestIntegrationLabelInvalidTaskId
**Setup:** Set TKIT_FILE (empty store, no tasks)
**Steps (use `t.Run()` formal subtests):**
1. Subtest "non-numeric": `t.Run("non-numeric", func(t *testing.T) { _, err := runCmd(t, "label", "add", "abc", "bug"); /* assert */ })` — `err != nil && strings.Contains(err.Error(), "invalid task ID")` (error message must contain exact substring "invalid task ID", case-sensitive)
2. Subtest "negative": `t.Run("negative", func(t *testing.T) { _, err := runCmd(t, "label", "add", "-1", "bug"); /* assert */ })` — `err != nil` (any error is acceptable, no specific message required)
3. Subtest "zero": `t.Run("zero", func(t *testing.T) { _, err := runCmd(t, "label", "add", "0", "bug"); /* assert */ })` — `err != nil && strings.Contains(err.Error(), "not found")` (error message must contain exact substring "not found", case-sensitive)

### TestIntegrationListFilterNoMatches
**Setup:** Set TKIT_FILE. Create a task with label "bug":
```go
runCmd(t, "add", "Bug task")
runCmd(t, "label", "add", "1", "bug")
```
**Input:** `out, err := runCmd(t, "list", "--label", "nonexistent")`
**Expected:** `err == nil`. `!strings.Contains(out, "Bug task")` — output does not contain any task titles. Verification: the output must not contain the string "Bug task". The output may be completely empty (`out == ""`), or may contain only whitespace or column headers, as long as no task content appears.

### TestIntegrationListFilterEmptyStore
**Setup:** Set TKIT_FILE (empty store, no tasks)
**Input:** `out, err := runCmd(t, "list", "--label", "bug")`
**Expected:** `err == nil`. Output contains no task content. Verification: the output may be completely empty (`out == ""`), or may contain only whitespace or column headers, as long as no task content appears.

### TestIntegrationRemoveNonexistentLabel
**Setup:** Set TKIT_FILE. Create a task via `runCmd(t, "add", "Test task")`.
**Input:** `_, err := runCmd(t, "label", "remove", "1", "nonexistent")`
**Expected:** `err == nil` — removing a label that doesn't exist is idempotent, not an error

### TestIntegrationMultipleLabelOperations
**Setup:** Set TKIT_FILE. Create a task.
**Steps:**
1. `runCmd(t, "label", "add", "1", "bug")`
2. `runCmd(t, "label", "add", "1", "frontend")`
3. `runCmd(t, "label", "remove", "1", "bug")`
4. `out, _ := runCmd(t, "show", "1")`
**Expected:** `strings.Contains(out, "frontend")` is true, `strings.Contains(out, "bug")` is false — only frontend remains

## Edge Cases

1. **Label deduplication end-to-end** — Add same label twice via CLI, verify stored only once
2. **Normalization end-to-end** — Add "BUG" via CLI, stored as "bug", show displays "bug"
3. **Empty label via CLI** — `label add 1 ""` returns error `"label cannot be empty"`
4. **Whitespace-only label** — `label add 1 "   "` normalizes to empty and returns error
5. **AND semantics for list --label** — Multiple `--label` flags require ALL labels present
6. **Filter with no matches** — Returns empty output, not an error
7. **Filter on empty store** — `list --label bug` on empty store returns empty output, not an error
8. **Remove non-existent label** — `label remove 1 nonexistent` should not error (idempotent)
9. **Invalid task ID variants** — Negative numbers, zero, non-numeric strings all handled gracefully
10. **Multiple sequential label operations** — Add A, add B, remove A leaves only B
11. **Persistence across command instances** — Labels added in one runCmd() call are visible in subsequent calls (disk I/O verification)
12. **Labeled task deletion** — Deleting a task removes its labels from `label list` aggregation
13. **Many labels per task** — Adding 15+ labels to a single task works correctly
14. **Shared label across tasks** — Same label on multiple tasks appears once in `label list`, all tasks appear in filtered list

## Integration Points

### Consumed by
- Nothing — this is the terminal phase

### Depends on
- Phase model: `model.Task.Labels` field, `model.NormalizeLabel`, `model.ValidateLabel`
- Phase store: `Store.AddLabel`, `Store.RemoveLabel`, `Store.ListLabels`
- Phase filter: `filter.Options.Labels` field
- Phase cli: `newLabelCmd()`, modified `newAddCmd()` with `-l` flag, modified `newListCmd()` with `--label` flag, modified `newShowCmd()` to display labels

### Exports
- None — this is the terminal phase
