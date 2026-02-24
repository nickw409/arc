# Phase: cli

## Objective

Add CLI commands and flags for label management: `--label` on `add`/`list`, new `label` subcommand group, and display labels in `show`.

## Files

First read these existing CLI files to understand the patterns used in this project:
- `internal/cli/root.go` — Store initialization and command registration
- `internal/cli/add.go` — Flag definitions, `RunE` pattern, error handling
- `internal/cli/show.go` — Task ID parsing, output formatting
- `internal/cli/list.go` — Filtering and output

**Key patterns you will discover:**
- `taskStore` is a **package-level** `*store.Store` variable, initialized in `NewRootCmd()`'s `PersistentPreRun` hook via `taskStore = store.New(storePath())`. All commands access `taskStore` directly.
- All commands use **`RunE`** (not `Run`), returning errors instead of calling `os.Exit`. Cobra prints returned errors to stderr automatically.
- `NewRootCmd()` takes **no parameters**. The store path comes from `TKIT_FILE` env var or defaults to `~/.tkit.json`.
- Task ID parsing uses `fmt.Errorf("invalid task ID: %s", args[0])` (not the raw strconv error).
- There are **no existing CLI test files**. This phase creates the first ones.

### Create
- `internal/cli/label.go` — New `label` command group with `add`, `remove`, `list` subcommands
- `internal/cli/label_test.go` — Tests for label CLI commands

### Modify
- `internal/cli/add.go` — Add `--label/-l` repeatable string slice flag
- `internal/cli/list.go` — Add `--label/-l` repeatable string slice flag, pass to `filter.Options.Labels`
- `internal/cli/show.go` — Display labels in task detail output
- `internal/cli/root.go` — Register `newLabelCmd()` via `root.AddCommand(newLabelCmd())`

## Types and Signatures

```go
// === label.go ===

// newLabelCmd returns the parent "label" command group.
func newLabelCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "label",
        Short: "Manage task labels",
    }
    cmd.AddCommand(newLabelAddCmd())
    cmd.AddCommand(newLabelRemoveCmd())
    cmd.AddCommand(newLabelListCmd())
    return cmd
}

// newLabelAddCmd returns the "label add <id> <label>" command.
func newLabelAddCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "add <id> <label>",
        Short: "Add a label to a task",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            id, err := strconv.Atoi(args[0])
            if err != nil {
                return fmt.Errorf("invalid task ID: %s", args[0])
            }
            if err := taskStore.AddLabel(id, args[1]); err != nil {
                return err
            }
            fmt.Printf("Added label %q to task #%d\n", args[1], id)
            return nil
        },
    }
}

// newLabelRemoveCmd returns the "label remove <id> <label>" command.
func newLabelRemoveCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "remove <id> <label>",
        Short: "Remove a label from a task",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            id, err := strconv.Atoi(args[0])
            if err != nil {
                return fmt.Errorf("invalid task ID: %s", args[0])
            }
            if err := taskStore.RemoveLabel(id, args[1]); err != nil {
                return err
            }
            fmt.Printf("Removed label %q from task #%d\n", args[1], id)
            return nil
        },
    }
}

// newLabelListCmd returns the "label list" command.
// GetAllLabels returns labels sorted alphabetically.
func newLabelListCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "list",
        Short: "List all labels",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            labels, err := taskStore.GetAllLabels()
            if err != nil {
                return err
            }
            for _, l := range labels {
                fmt.Println(l)
            }
            return nil
        },
    }
}

// === add.go modifications ===
// In newAddCmd(), add a labels flag and call AddLabel after task creation:
//
//   var labels []string
//   // ... existing flag definitions ...
//   cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Add labels to the task")
//
//   // Inside RunE, after the success message (fmt.Printf("Added task #%d: %s\n", task.ID, task.Title)):
//   for _, label := range labels {
//       if err := taskStore.AddLabel(task.ID, label); err != nil {
//           return err  // Stop on first label error; task is created and success message printed, but only prior labels are saved
//       }
//   }

// === list.go modifications ===
// In newListCmd(), add a labels flag and pass to filter options:
//
//   var labels []string
//   cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Filter by label (AND semantics)")
//
//   // Inside RunE, when building filter options:
//   opts := filter.Options{
//       // ... existing fields ...
//       Labels: labels,
//   }
//   // NOTE: filtering via filter.Options, NOT via Store.ListByLabel

// === show.go modifications ===
// In newShowCmd() RunE, after existing fmt.Printf lines for fields, add:
//
//   if len(task.Labels) > 0 {
//       fmt.Printf("  Labels:    %s\n", strings.Join(task.Labels, ", "))
//   }
//   // Add "strings" to the import block if not already present.
//   // The format string "  Labels:    %s\n" has exactly 2 leading spaces, then "Labels:", then exactly 4 spaces, then %s.
//   // Labels display in the order they appear in task.Labels (the order they were added to the task), comma-separated.
//   // If task has no labels (nil or empty slice), no Labels line is printed.

// === root.go modifications ===
// In NewRootCmd(), after existing AddCommand calls, add:
//   root.AddCommand(newLabelCmd())
```

## Error Types

```go
// All commands use RunE — return errors, do NOT call os.Exit(1).
// Cobra prints returned errors to stderr and exits with code 1 automatically.
//
// Error patterns (match existing code in show.go, add.go):
//   Invalid task ID:  return fmt.Errorf("invalid task ID: %s", args[0])
//   Store errors:     return err  (propagated directly from store methods)
//   Validation errors: return err (propagated from store → model.ValidateLabel)
```

## Dependencies

None new — uses `github.com/spf13/cobra` (already a dependency).

**Hard dependencies on previous phases:**
- Phase model: `model.ValidateLabel(s string) error` — called internally by `Store.AddLabel`
- Phase store: `Store.AddLabel`, `Store.RemoveLabel`, `Store.GetAllLabels` — must exist
- Phase filter: `filter.Options.Labels` field — must exist

Before writing any code in this phase, verify these dependencies exist by reading:
1. `internal/store/store.go` — confirm AddLabel, RemoveLabel, GetAllLabels methods exist
2. `internal/filter/filter.go` — confirm Options.Labels field exists

## DO NOT

- [ ] Do NOT modify `internal/model/task.go`
- [ ] Do NOT modify `internal/store/store.go`
- [ ] Do NOT modify `internal/filter/filter.go`
- [ ] Do NOT change existing command names, aliases, or flag names
- [ ] Do NOT change how `taskStore` is initialized (it's a package-level var set in `PersistentPreRun`)
- [ ] Do NOT use `os.Exit(1)` — use `RunE` and return errors (cobra handles exit codes)
- [ ] Do NOT call `Store.ListByLabel` from the `list` command — use `filter.Options.Labels` instead
- [ ] Do NOT normalize labels in the CLI — the store handles normalization

## Test Cases

All test names use Go's `TestXxx` convention.

**Testing pattern:** Since there are no existing CLI tests in this project, use this approach:
1. Set `TKIT_FILE` env var to a temp file path: `t.Setenv("TKIT_FILE", filepath.Join(t.TempDir(), "tasks.json"))`
2. Build root command: `cmd := NewRootCmd()`
3. Set args: `cmd.SetArgs([]string{"label", "add", "1", "bug"})`
4. Capture output: `var buf bytes.Buffer; cmd.SetOut(&buf); cmd.SetErr(&buf)`
5. Execute: `err := cmd.Execute()`
6. Assert on `buf.String()` for output, `err` for errors
7. For store verification: `s := store.New(tempFilePath)` where tempFilePath is the same path used in TKIT_FILE, then call `s.Get(1)` etc.

Each test creates a fresh temp file via `t.TempDir()`.

### TestLabelAddCmd
**Setup:** Set TKIT_FILE to temp path. Execute `add "Test task"` first.
**Input:** Execute `label add 1 bug`
**Expected:** `err == nil`. Use `strings.Contains(buf.String(), `Added label "bug" to task #1`)` to verify output (there may be additional output before or after this substring). Then verify: `store.New(path).Get(1)` has `Labels: []string{"bug"}`

### TestLabelAddCmdNormalizes
**Setup:** Set TKIT_FILE, execute `add "Test task"`
**Input:** Execute `label add 1 "  BUG  "`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug"}`

### TestLabelRemoveCmd
**Setup:** Set TKIT_FILE, execute `add "Test task"`, then `label add 1 bug`
**Input:** Execute `label remove 1 bug`
**Expected:** `err == nil`. Output contains `Removed label "bug" from task #1`.

### TestLabelRemoveNonexistentLabel
**Setup:** Set TKIT_FILE, execute `add "Test task"` (no labels)
**Input:** Execute `label remove 1 nonexistent`
**Expected:** `err == nil` (no-op, store.RemoveLabel returns nil for absent labels)

### TestLabelRemoveNonexistentTask
**Setup:** Set TKIT_FILE (no tasks)
**Input:** Execute `label remove 999 bug`
**Expected:** `err != nil`. Error message contains `"task 999 not found"`.

### TestLabelRemoveFromMultiple
**Setup:** Set TKIT_FILE, add task, add labels "bug", "frontend", "urgent"
**Input:** Execute `label remove 1 frontend`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug", "urgent"}`

### TestLabelListCmd
**Setup:** Set TKIT_FILE, add 2 tasks, add label "frontend" to task 1, "bug" to task 2
**Input:** Execute `label list`
**Expected:** `err == nil`. Output is exactly `"bug\nfrontend\n"`. Store.GetAllLabels returns labels sorted alphabetically (not insertion order).

### TestLabelListEmpty
**Setup:** Set TKIT_FILE, add a task (no labels)
**Input:** Execute `label list`
**Expected:** `err == nil`. Output is empty string.

### TestLabelCmdWithoutSubcommand
**Setup:** Set TKIT_FILE
**Input:** Execute `label` (no subcommand)
**Expected:** Help text is displayed (verify command structure is valid). Output contains "Manage task labels".

### TestLabelCmdInvalidSubcommand
**Setup:** Set TKIT_FILE
**Input:** Execute `label invalid`
**Expected:** `err != nil`. Error message indicates unknown subcommand.

### TestLabelCmdRegisteredInRoot
**Setup:** Set TKIT_FILE
**Input:** Build root command, get all subcommands via `cmd.Commands()`, search for command with `Use == "label"`
**Expected:** Label command is found in root's subcommands.

### TestLabelAddWithSpecialCharacters
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "bug:critical"`
**Expected:** `err == nil`. `store.New(path).Get(1)` has the label (verify special chars in labels are allowed and normalized correctly)

### TestLabelAddWithUnicode
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "优先级"`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"优先级"}` (verify unicode support)

### TestLabelAddNumericOnly
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "123"`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"123"}`

### TestLabelAddMixedCase
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "BugFix"`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bugfix"}` (verify lowercase normalization)

### TestLabelAddVeryLong
**Setup:** Set TKIT_FILE, add a task. Create label string of 1000 characters.
**Input:** Execute `label add 1 <1000-char-string>`
**Expected:** `err == nil`. Label is stored (or `err != nil` if store has length limit - verify behavior)

### TestLabelAddSingleCharacter
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "a"`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"a"}`

### TestAddFlagAfterPositional
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" --label bug`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug"}` (verify flag ordering)

### TestAddFlagMixedShortLong
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" -l bug --label frontend`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug", "frontend"}` (verify short/long flag mixing)

### TestListWithLabelFilterNoResults
**Setup:** Set TKIT_FILE. Add task with label "bug"
**Input:** Execute `list --label nonexistent`
**Expected:** `err == nil`. Output is empty or contains no tasks.

### TestShowWithManyLabels
**Setup:** Set TKIT_FILE, add a task, add 20 different labels
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output contains `Labels:` line with all 20 labels comma-separated (verify formatting handles many labels)

### TestAddWithLabelFlag
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" -l bug -l frontend`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug", "frontend"}`

### TestAddWithLabelLongForm
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" --label bug`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug"}`

### TestAddWithNoLabels
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task"` (no -l flags)
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: nil` or `Labels: []string{}`

### TestAddWithDuplicateLabels
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" -l bug -l bug`
**Expected:** `err == nil`. `store.New(path).Get(1)` has `Labels: []string{"bug"}` (deduplicated by store)

### TestAddWithLabelFlagErrorStops
**Setup:** Set TKIT_FILE
**Input:** Execute `add "Test task" -l bug -l "" -l frontend`
**Expected:** `err != nil`, error message contains `"label cannot be empty"`. Output contains the task creation success message (`Added task #1: Test task`). Task 1 exists with `Labels: []string{"bug"}` only (stopped on first label error after printing success).

### TestListWithLabelFilter
**Setup:** Set TKIT_FILE. Add 3 tasks: "Bug task" (label "bug"), "Feature task" (label "feature"), "Both task" (labels "bug", "frontend")
**Input:** Execute `list --label bug`
**Expected:** `err == nil`. Output contains "Bug task" and "Both task". Does NOT contain "Feature task".

### TestListWithLabelFilterLongForm
**Setup:** Set TKIT_FILE. Add task with label "bug"
**Input:** Execute `list --label bug`
**Expected:** `err == nil`. Output contains the task.

### TestListWithDuplicateLabelFilters
**Setup:** Set TKIT_FILE. Add task with label "bug"
**Input:** Execute `list --label bug --label bug`
**Expected:** `err == nil`. Output contains the task (duplicate filter values handled correctly)

### TestListWithThreeLabelFilters
**Setup:** Set TKIT_FILE. Add task with labels "bug", "frontend", "urgent"
**Input:** Execute `list --label bug --label frontend --label urgent`
**Expected:** `err == nil`. Output contains the task (all three labels match)

### TestListWithMultipleLabelFilters
**Setup:** Same 3 tasks as above
**Input:** Execute `list --label bug --label frontend`
**Expected:** `err == nil`. Output contains only "Both task" (AND semantics: must have both labels).

### TestShowDisplaysLabels
**Setup:** Set TKIT_FILE, add a task, add labels "bug" and "frontend"
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output contains substring `Labels:    bug, frontend`

### TestShowLabelsWithCommaInValue
**Setup:** Set TKIT_FILE, add a task, add label "bug,critical" (if allowed by validation)
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output displays the label correctly (verify comma handling in output)

### TestShowManyLabelsFormatting
**Setup:** Set TKIT_FILE, add a task, add labels in specific order: "aaa", "zzz", "mmm"
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output contains `Labels:    aaa, zzz, mmm` (verify display preserves stored order, not sorted)

### TestShowNoLabelsLine
**Setup:** Set TKIT_FILE, add a task (no labels)
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output does NOT contain the string `Labels:`

### TestShowSingleLabel
**Setup:** Set TKIT_FILE, add a task, add label "bug"
**Input:** Execute `show 1`
**Expected:** `err == nil`. Output contains substring `Labels:    bug`

### TestLabelAddInvalidTaskId
**Setup:** Set TKIT_FILE
**Input:** Execute `label add notanumber bug`
**Expected:** `err != nil`. Error message contains `"invalid task ID"`.

### TestLabelAddTaskIdZero
**Setup:** Set TKIT_FILE
**Input:** Execute `label add 0 bug`
**Expected:** `err != nil`. Error message contains `"task 0 not found"` (or similar store error)

### TestLabelAddNegativeTaskId
**Setup:** Set TKIT_FILE
**Input:** Execute `label add -1 bug`
**Expected:** `err != nil`. Error message contains `"invalid task ID"` or store-level error.

### TestLabelAddVeryLargeTaskId
**Setup:** Set TKIT_FILE
**Input:** Execute `label add 2147483647 bug`
**Expected:** `err != nil`. Error message contains `"task 2147483647 not found"`

### TestLabelAddNonexistentTask
**Setup:** Set TKIT_FILE (no tasks)
**Input:** Execute `label add 999 bug`
**Expected:** `err != nil`. Error message contains `"task 999 not found"`.

### TestLabelRemoveInvalidTaskId
**Setup:** Set TKIT_FILE
**Input:** Execute `label remove notanumber bug`
**Expected:** `err != nil`. Error message contains `"invalid task ID"`.

### TestLabelAddEmptyLabel
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 ""`
**Expected:** `err != nil`. Error message contains `"label cannot be empty"`.

### TestLabelAddWhitespaceOnly
**Setup:** Set TKIT_FILE, add a task
**Input:** Execute `label add 1 "   "`
**Expected:** `err != nil`. Error message contains `"label cannot be empty"` (whitespace normalizes to empty, fails validation)

### TestLabelAddDuplicate
**Setup:** Set TKIT_FILE, add a task, execute `label add 1 bug`
**Input:** Execute `label add 1 bug` again
**Expected:** `err == nil` (idempotent, store deduplicates). `store.New(path).Get(1)` has `Labels: []string{"bug"}`

### TestLabelAddWrongArgCount
**Input:** Execute `label add 1` (missing label argument)
**Expected:** `err != nil`. Cobra rejects (ExactArgs(2) violation).

### TestLabelAddTooManyArgs
**Input:** Execute `label add 1 bug extra`
**Expected:** `err != nil`. Cobra rejects (ExactArgs(2) violation).

### TestLabelRemoveWrongArgCount
**Input:** Execute `label remove 1` (missing label argument)
**Expected:** `err != nil`. Cobra rejects (ExactArgs(2) violation).

### TestLabelRemoveTooManyArgs
**Input:** Execute `label remove 1 bug extra`
**Expected:** `err != nil`. Cobra rejects (ExactArgs(2) violation).

### TestLabelListWithArgs
**Input:** Execute `label list extra`
**Expected:** `err != nil`. Cobra rejects (NoArgs violation).

## Edge Cases

1. **Invalid task ID** — `label add` and `label remove` return `fmt.Errorf("invalid task ID: %s", args[0])`
2. **Empty label argument** — Propagated from `Store.AddLabel` → `model.ValidateLabel` → `"label cannot be empty"`
3. **No labels on task** — `show` omits the Labels line entirely
4. **Add flag partial failure** — `-l bug -l ""` creates the task and adds "bug", then returns error on ""; does not roll back
5. **AND semantics for --label** — Multiple `--label` flags require ALL labels (from filter phase)
6. **Duplicate label** — `label add` with existing label is a no-op (returns nil)

## Integration Points

### Consumed by
- End users via the CLI
- Phase integration: Integration tests exercise these commands

### Depends on
- Phase model: `model.ValidateLabel(s string) error` (called by Store.AddLabel internally)
- Phase store: `Store.AddLabel(taskID int, label string) error`, `Store.RemoveLabel(taskID int, label string) error`, `Store.GetAllLabels() ([]string, error)`
- Phase filter: `filter.Options.Labels` field (`[]string`) — used by `list` command

### Exports
- `newLabelCmd() *cobra.Command` — registered in root.go
- Modified `newAddCmd()` — now accepts `-l` flag
- Modified `newListCmd()` — now accepts `--label` flag
- Modified `newShowCmd()` — now displays labels
