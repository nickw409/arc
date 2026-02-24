# Task 1: Implementation Plan — Label/Tag Support

## Phase 1: Data Model Changes
1. Add `Labels []string` field to `model.Task` struct with `json:"labels,omitempty"` tag
2. Add helper functions in model package:
   - `NormalizeLabel(s string) string` — lowercase, trim whitespace
   - `ValidateLabel(s string) error` — reject empty strings
3. Write tests for the helpers

## Phase 2: Store Layer
1. Add `AddLabel(taskID int, label string) error` to store
2. Add `RemoveLabel(taskID int, label string) error` to store
3. Add `ListLabels() ([]string, error)` to store — returns all unique labels
4. Add `ListByLabel(label string) ([]model.Task, error)` to store
5. Write tests for each new method

## Phase 3: Filter Integration
1. Add `Labels []string` field to `filter.Options`
2. Update `filter.Apply` to filter by labels (task must have ALL specified labels)
3. Write filter tests for label filtering

## Phase 4: CLI Commands
1. Modify `add` command: add `--label/-l` StringSliceVar flag, normalize and validate before passing to store
2. Modify `list` command: add `--label/-l` StringSliceVar flag, pass to filter options
3. Create `label` command group:
   - `label add <id> <label>` — calls store.AddLabel
   - `label remove <id> <label>` — calls store.RemoveLabel
   - `label list` — calls store.ListLabels, prints unique labels
4. Modify `show` command: display labels if present

## Phase 5: Verification
1. Run all existing tests — ensure no regressions
2. Run new tests
3. Manual smoke test of CLI commands
