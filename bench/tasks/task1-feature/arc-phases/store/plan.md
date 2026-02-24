# Phase: store

## Objective

Add label management methods to the Store so tasks can have labels added, removed, and queried.

## Files

### Modify
- `internal/store/store.go` — Add `AddLabel`, `RemoveLabel`, `ListLabels`, and `ListByLabel` methods

### Create
- `internal/store/label_test.go` — Unit tests for all label store methods

## Types and Signatures

Before implementing any methods:

1. Read `internal/store/store.go` to verify the existing load/save pattern.
2. Confirm that `s.load()` returns `([]model.Task, error)` and `s.save(tasks []model.Task) error` exist as methods.
3. Examine at least one existing public method (e.g., `Store.Get` or `Store.Complete`) to confirm the pattern: call `s.load()`, operate on the slice, call `s.save(tasks)`.
4. Identify the exact error message format used for "task not found" errors in existing methods. The expected format is `fmt.Errorf("task %d not found", taskID)`.

If the existing code uses this exact format, proceed with implementation. If the existing code uses a different format, STOP and report: "Store error format mismatch — existing code uses [ACTUAL FORMAT], expected format is 'task %d not found'. Need clarification on which format to use."

Follow this exact pattern for all new methods: every public method calls `s.load()` to read all tasks from the JSON file, operates on the in-memory slice, then calls `s.save(tasks)` to write back.

```go
// AddLabel adds a label to the task with the given ID.
// Order of operations:
//   1. Normalize: normalized := model.NormalizeLabel(label)
//   2. Call model.ValidateLabel(normalized) — return error immediately if invalid
//   3. Call s.load() to get tasks
//   4. Find task by ID — return fmt.Errorf("task %d not found", taskID) if not found
//   5. Check if normalized label already exists in task.Labels — if yes, return nil (no-op)
//   6. Append normalized label to task.Labels (append order — new labels go at end)
//   7. Call s.save(tasks)
func (s *Store) AddLabel(taskID int, label string) error

// RemoveLabel removes a label from the task with the given ID.
// Unlike AddLabel, this method does NOT call model.ValidateLabel — it only normalizes.
// Invalid or empty labels after normalization simply won't match any existing labels (no-op).
// Order of operations:
//   1. Normalize: normalized := model.NormalizeLabel(label)
//   2. Do NOT call model.ValidateLabel (skip validation entirely)
//   3. Call s.load() to get tasks
//   4. Find task by ID — return fmt.Errorf("task %d not found", taskID) if not found
//   5. Store original length: originalLen := len(task.Labels)
//   6. Rebuild task.Labels as a new slice: create newLabels := []string{}, iterate through task.Labels, and for each existing label, if it does NOT equal normalized (case-sensitive string comparison), append it to newLabels. This preserves order of remaining labels.
//   7. Compare len(newLabels) to originalLen. If lengths are equal, the label was not present — return nil without calling s.save (no-op).
//   8. If lengths differ, assign newLabels to task.Labels and call s.save(tasks).
func (s *Store) RemoveLabel(taskID int, label string) error

// ListLabels returns all unique labels across all tasks, sorted alphabetically.
// Implementation: collect all labels from all tasks (each label is already normalized/lowercase from AddLabel), deduplicate into a set, convert to slice, sort using sort.Strings (lexicographic byte order).
// Returns []string{} (not nil) when no labels exist.
func (s *Store) ListLabels() ([]string, error)

// ListByLabel returns all tasks that have the given label.
// The label is normalized via model.NormalizeLabel before comparison.
// Returns tasks in the order they appear in the in-memory slice returned by s.load() (which reflects their position in the JSON file's task array — typically this is the order tasks were added to the store, preserved by the ID assignment).
// Returns []model.Task{} (not nil) when no tasks match.
func (s *Store) ListByLabel(label string) ([]model.Task, error)
```

## Error Types

```go
// Task not found (reuses existing store pattern — verify by reading store.go):
fmt.Errorf("task %d not found", taskID)

// Label validation (propagated directly from model.ValidateLabel):
fmt.Errorf("label cannot be empty")
```

## Dependencies

None new — uses `model.NormalizeLabel` and `model.ValidateLabel` from the model phase.

**Hard dependency:** Phase model must be complete. Before writing any code:

1. Read `internal/model/task.go` and verify the following symbols exist:
   - `model.Task.Labels` field of type `[]string`
   - `model.NormalizeLabel(s string) string` function
   - `model.ValidateLabel(s string) error` function

2. If ANY of these symbols are missing, STOP and report: "Phase model incomplete — cannot proceed with store phase until model.NormalizeLabel, model.ValidateLabel, and model.Task.Labels are implemented."

3. Only proceed to implementation if all three symbols are verified to exist.

## DO NOT

- [ ] Do NOT modify `internal/model/task.go` — that was done in the model phase
- [ ] Do NOT modify `internal/filter/filter.go` — that is a later phase
- [ ] Do NOT add CLI commands — that is a later phase
- [ ] Do NOT change the signatures of any existing Store methods
- [ ] Do NOT add caching or change the load/save pattern (the store reads from disk on every method call — leave this as-is)
- [ ] Do NOT call `RemoveLabel` with validation — unlike `AddLabel`, `RemoveLabel` does not validate (it just normalizes)

## Test Cases

### TestAddLabel
**Setup:** 
1. Create store: `s := store.New(filepath.Join(t.TempDir(), "tasks.json"))`
2. Add a task: `taskID, err := s.Add("task1", "", model.PriorityNone)` — verify err is nil and store taskID in a variable

**Input:** `s.AddLabel(taskID, "bug")` (use the actual returned ID stored in taskID variable)
**Expected:** Returns `nil`. `s.Get(taskID)` returns task with `Labels: []string{"bug"}`

Note: For all test cases below that reference task IDs, use the actual task IDs returned by `s.Add()` and stored in variables (e.g., `taskID1`, `taskID2`, `taskID3`), not hardcoded literal values like 1, 2, 3. The documentation below uses 1, 2, 3 for readability, but implementation must use the actual returned IDs.

### TestAddLabelNormalizes
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "  BUG  ")`
**Expected:** Returns `nil`. `s.Get(1)` returns task with `Labels: []string{"bug"}`

### TestAddLabelDuplicateIgnored
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "bug")` then `s.AddLabel(1, "bug")` again
**Expected:** Both return `nil`. `s.Get(1)` returns task with `Labels: []string{"bug"}` (no duplicate)

### TestAddLabelDuplicateDifferentCase
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "bug")` then `s.AddLabel(1, "BUG")`
**Expected:** Both return `nil`. `s.Get(1)` returns task with `Labels: []string{"bug"}` (normalized "BUG" → "bug" detected as duplicate)

### TestAddLabelMultiple
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "bug")` then `s.AddLabel(1, "frontend")`
**Expected:** `s.Get(1)` returns task with `Labels: []string{"bug", "frontend"}` (append order preserved)

### TestAddLabelInvalidEmpty
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "")`
**Expected:** Returns error with message exactly `"label cannot be empty"`

### TestAddLabelOnNilLabelsField
**Setup:** Create store with one task using `s.Add("task1", "", model.PriorityNone)`. Use the actual returned task ID (store in variable `taskID`). Verify precondition: `task, _ := s.Get(taskID)` should have `task.Labels == nil` (not initialized to empty slice).
**Input:** `s.AddLabel(taskID, "bug")`
**Expected:** Returns `nil`. `s.Get(taskID)` returns task with `Labels: []string{"bug"}` (append to nil slice must work correctly — Go append handles nil slices)

### TestAddLabelTaskNotFound
**Input:** `s.AddLabel(999, "bug")` on empty store
**Expected:** Returns error with message `"task 999 not found"`

### TestAddLabelInvalidLabelBeforeTaskLookup
**Input:** `s.AddLabel(999, "")` on empty store
**Expected:** Returns error with message `"label cannot be empty"` (validation checked FIRST, before task lookup)

### TestAddLabelWhitespaceOnly
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "  ")`
**Expected:** Returns error with message `"label cannot be empty"` (whitespace normalizes to empty, fails validation)

### TestAddLabelUnicode
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "bug🐛")`
**Expected:** Returns `nil`. `s.Get(1)` returns task with label containing the emoji (verify JSON serialization handles Unicode)

### TestAddLabelSpecialCharacters
**Setup:** Create store with one task
**Input:** `s.AddLabel(1, "front/end")`
**Expected:** Returns `nil`. `s.Get(1)` returns task with `Labels: []string{"front/end"}` (special characters preserved, normalized to lowercase)

### TestListByLabelWhitespaceOnly
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`
**Input:** `s.ListByLabel("   ")`
**Expected:** Returns `[]model.Task{}` (whitespace normalizes to "", no label matches)

### TestRemoveLabel
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`
**Input:** `s.RemoveLabel(1, "bug")`
**Expected:** Returns `nil`. `s.Get(1)` returns task where `len(task.Labels) == 0` (may be nil slice or empty slice, both acceptable)

### TestRemoveLabelPreservesOthers
**Setup:** Create store with one task, add labels "bug", "frontend", "urgent"
**Input:** `s.RemoveLabel(1, "frontend")`
**Expected:** Returns `nil`. `s.Get(1)` returns task with `Labels: []string{"bug", "urgent"}` (order preserved, removed label gone)

### TestRemoveLabelNotPresent
**Setup:** Create store with one task (no labels)
**Input:** `s.RemoveLabel(1, "nonexistent")`
**Expected:** Returns `nil` (no error, no-op)

### TestRemoveLabelNormalizes
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`
**Input:** `s.RemoveLabel(1, "BUG")`
**Expected:** Returns `nil`. `s.Get(1)` returns task with empty/nil Labels

### TestRemoveLabelTaskNotFound
**Input:** `s.RemoveLabel(999, "bug")` on empty store
**Expected:** Returns error with message `"task 999 not found"`

### TestRemoveLabelNegativeTaskID
**Input:** `s.RemoveLabel(-1, "bug")` on empty store
**Expected:** Returns error with message `"task -1 not found"`

### TestRemoveLabelZeroTaskID
**Input:** `s.RemoveLabel(0, "bug")` on empty store
**Expected:** Returns error with message `"task 0 not found"`

### TestRemoveLabelEmptyString
**Setup:** Create store with one task, add label "bug"
**Input:** `s.RemoveLabel(1, "")`
**Expected:** Returns nil (normalizes to "", no label matches, no-op)

### TestRemoveLabelMultipleSequential
**Setup:** Create store with one task, add labels "bug", "frontend", "urgent"
**Input:** `s.RemoveLabel(1, "bug")`, then `s.RemoveLabel(1, "frontend")`, then `s.RemoveLabel(1, "urgent")`
**Expected:** All return `nil`. Final `s.Get(1)` returns task with `Labels: []string{}` or nil

### TestListLabels
**Setup:** Create two tasks, `s.AddLabel(1, "bug")`, `s.AddLabel(2, "bug")`, `s.AddLabel(2, "frontend")`
**Expected:** `s.ListLabels()` returns `[]string{"bug", "frontend"}` (sorted alphabetically, deduplicated)

### TestListLabelsEmpty
**Setup:** Empty store OR store with tasks that have no labels
**Expected:** `s.ListLabels()` returns `[]string{}` (empty slice, not nil), no error

### TestListLabelsEmptyStore
**Setup:** Empty store (no tasks at all)
**Expected:** `s.ListLabels()` returns `[]string{}` (empty slice, not nil), no error

### TestListLabelsMultiTaskDedup
**Setup:** Create two tasks. `s.AddLabel(1, "Bug")`, `s.AddLabel(2, "bug")`, `s.AddLabel(2, "frontend")`
**Expected:** `s.ListLabels()` returns `[]string{"bug", "frontend"}` — "Bug" from task 1 was stored as "bug" (normalized by AddLabel), deduplicated with task 2's "bug"

### TestListLabelsSortOrder
**Setup:** Create one task. `s.AddLabel(1, "zebra")`, `s.AddLabel(1, "alpha")`, `s.AddLabel(1, "123numeric")`
**Expected:** `s.ListLabels()` returns labels sorted lexicographically: `[]string{"123numeric", "alpha", "zebra"}`

### TestListByLabel
**Setup:** Create two tasks, add "bug" to both, add "frontend" to task 2 only
**Expected:** `s.ListByLabel("bug")` returns both tasks (IDs 1, 2). `s.ListByLabel("frontend")` returns only task 2.

### TestListByLabelNormalizes
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`
**Input:** `s.ListByLabel("BUG")`
**Expected:** Returns slice containing task 1

### TestListByLabelMultiLabelTask
**Setup:** Create store with one task. `s.AddLabel(1, "bug")`, `s.AddLabel(1, "frontend")`
**Input:** `s.ListByLabel("bug")`
**Expected:** Returns slice containing task 1 (task has multiple labels, query matches one of them)

### TestListByLabelOrderingThreeTasks
**Setup:** Create three tasks by calling `s.Add` three times (IDs will be 1, 2, 3). Add label "bug" to all three.
**Input:** `s.ListByLabel("bug")`
**Expected:** Returns tasks in order [1, 2, 3] (the order they appear in the in-memory slice from s.load(), which matches the order they were added)

### TestListByLabelNoMatches
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`
**Input:** `s.ListByLabel("nonexistent")`
**Expected:** Returns `[]model.Task{}` (empty slice, not nil), no error

### TestListByLabelEmptyStore
**Setup:** Empty store (no tasks)
**Input:** `s.ListByLabel("bug")`
**Expected:** Returns `[]model.Task{}` (empty slice, not nil), no error

### TestListByLabelEmptyString
**Setup:** Create store with one task, add label "bug"
**Input:** `s.ListByLabel("")`
**Expected:** Returns `[]model.Task{}` (normalizes to "", no label matches)

### TestLabelsPersistAcrossLoad
**Setup:** Create store with one task, `s.AddLabel(1, "bug")`, `s.AddLabel(1, "frontend")`
**Input:** Create a NEW Store instance pointing to the same file: `s2 := store.New(samePath)`, then `s2.Get(1)`
**Expected:** Task has `Labels: []string{"bug", "frontend"}` — labels survive JSON round-trip through save/load

## Edge Cases

1. **Task with no labels** — `RemoveLabel` returns nil (no-op). `Get` returns task with `len(task.Labels) == 0` (Labels field may be nil or empty slice).
2. **Multiple labels on one task** — Append order preserved. Persist correctly through JSON save/load cycle.
3. **Label normalization in queries** — `ListByLabel("BUG")` matches tasks with stored label `"bug"`
4. **Duplicate add is idempotent** — `AddLabel` with existing label returns nil without modifying the slice
5. **Remove preserves order** — Removing a middle label does not reorder remaining labels

## Integration Points

### Consumed by
- Phase cli: `label add` calls `Store.AddLabel`, `label remove` calls `Store.RemoveLabel`, `label list` calls `Store.ListLabels`. Note: `Store.ListByLabel` is exported for programmatic use but is NOT called by the CLI's `list` command (which uses `filter.Options.Labels` instead).
- Phase filter: Does NOT call store methods — filtering happens via `filter.Apply` on the task list

### Depends on
- Phase model: `model.NormalizeLabel(s string) string`, `model.ValidateLabel(s string) error`, `model.Task.Labels` field (`[]string`)

### Exports
- `Store.AddLabel(taskID int, label string) error` — called by cli phase
- `Store.RemoveLabel(taskID int, label string) error` — called by cli phase
- `Store.ListLabels() ([]string, error)` — called by cli phase
- `Store.ListByLabel(label string) ([]model.Task, error)` — available for programmatic use (not called by CLI)
