# Phase: filter

## Objective

Add label filtering to the filter package so `list` can filter tasks by one or more labels.

## Files

### Modify
- `internal/filter/filter.go` — Add `Labels` field to `Options`, add label filtering pass to `Apply`

### Create
- `internal/filter/label_filter_test.go` — Unit tests for label filtering (absolute path from project root: `testbed/internal/filter/label_filter_test.go`)

## Types and Signatures

First read `internal/filter/filter.go` to understand the existing pattern. The `Apply` function runs separate filtering passes for Status, Priority, and Query. Add a new pass for Labels following the same pattern.

```go
// Modified Options struct — add Labels field after Query:
type Options struct {
    Status   *model.Status
    Priority *model.Priority
    Query    string
    Labels   []string  // Filter to tasks that have ALL of these labels (AND semantics)
}

// Apply signature does NOT change: func Apply(tasks []model.Task, opts Options) []model.Task
// Add a new filtering pass after the existing Query pass:
//
//   // Filter by labels (separate pass) — AND semantics: task must have ALL specified labels
//   if len(opts.Labels) > 0 {
//       var filtered []model.Task
//       for _, t := range result {
//           if hasAllLabels(t, opts.Labels) {
//               filtered = append(filtered, t)
//           }
//       }
//       result = filtered
//   }
//
// Helper (unexported):
// func hasAllLabels(t model.Task, required []string) bool
//   Implementation: iterate over required slice. For each required label, iterate over t.Labels and check if any element matches using == operator.
//   If any required label is not found in t.Labels, return false.
//   If all required labels are found, return true.
//   If t.Labels is nil, return false for any non-empty required list.
//   Both nil and []string{} for opts.Labels mean no filtering (checked by len == 0).
//   Do NOT deduplicate the required slice — check labels as provided.
```

## Error Types

None — the filter package does not return errors; it filters in-place.

## Dependencies

**Hard dependency:** Phase model must be complete. `model.Task.Labels` field must exist for this code to compile.

**Pre-execution verification:**
1. Read `internal/model/task.go`
2. Verify that `type Task struct` contains a `Labels []string` field
3. If field does NOT exist: STOP and report "Phase model dependency not met — Task.Labels field missing"
4. If field exists: proceed with implementation

## DO NOT

- [ ] Do NOT modify `internal/model/task.go`
- [ ] Do NOT modify `internal/store/store.go`
- [ ] Do NOT modify any CLI files
- [ ] Do NOT change the signature of `Apply`, `SortByPriority`, or `SortByDate`
- [ ] Do NOT change the existing filtering logic for Status, Priority, or Query
- [ ] Do NOT normalize labels in the filter — labels on `Task.Labels` are already normalized by the store phase

## Test Cases

All test names use Go's `TestXxx` convention.

### TestFilterBySingleLabel
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"bug"}},
    {ID: 2, Labels: []string{"feature"}},
    {ID: 3, Labels: []string{"bug", "frontend"}},
}
opts := Options{Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns tasks with IDs `[1, 3]`

### TestFilterByMultipleLabelsAndSemantics
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"bug"}},
    {ID: 2, Labels: []string{"bug", "frontend"}},
    {ID: 3, Labels: []string{"bug", "frontend", "urgent"}},
}
opts := Options{Labels: []string{"bug", "frontend"}}
```
**Expected:** `Apply(tasks, opts)` returns tasks with IDs `[2, 3]` (AND: must have ALL specified labels)

### TestFilterLabelsWithStatusFilter
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Status: model.StatusActive, Labels: []string{"bug"}},
    {ID: 2, Status: model.StatusCompleted, Labels: []string{"bug"}},
    {ID: 3, Status: model.StatusActive, Labels: []string{"feature"}},
}
active := model.StatusActive
opts := Options{Status: &active, Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1`

### TestFilterLabelsWithPriorityFilter
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Priority: model.PriorityHigh, Labels: []string{"bug"}},
    {ID: 2, Priority: model.PriorityLow, Labels: []string{"bug"}},
}
high := model.PriorityHigh
opts := Options{Priority: &high, Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1`

### TestFilterNoLabelsNil
**Input:**
```go
tasks := []model.Task{{ID: 1, Labels: []string{"bug"}}, {ID: 2}}
opts := Options{Labels: nil}
```
**Expected:** `Apply(tasks, opts)` returns all tasks (no label filter applied)

### TestFilterNoLabelsEmptySlice
**Input:**
```go
tasks := []model.Task{{ID: 1, Labels: []string{"bug"}}, {ID: 2}}
opts := Options{Labels: []string{}}
```
**Expected:** `Apply(tasks, opts)` returns all tasks (empty slice treated same as nil)

### TestFilterLabelNoMatches
**Input:**
```go
tasks := []model.Task{{ID: 1, Labels: []string{"bug"}}}
opts := Options{Labels: []string{"nonexistent"}}
```
**Expected:** `Apply(tasks, opts)` returns a nil slice (NOT `[]model.Task{}`)

### TestFilterTaskWithNilLabels
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: nil},
    {ID: 2, Labels: []string{"bug"}},
}
opts := Options{Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `2` (nil Labels never matches)

### TestFilterEmptyTaskList
**Input:**
```go
tasks := []model.Task{}
opts := Options{Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns empty/nil slice

### TestFilterTaskWithEmptyLabelsSlice
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{}},   // Empty slice, not nil
    {ID: 2, Labels: []string{"bug"}},
}
opts := Options{Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `2` (empty slice behaves like nil — no labels match)

### TestFilterDuplicateLabelsInCriteria
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"bug"}},
    {ID: 2, Labels: []string{"bug", "frontend"}},
}
opts := Options{Labels: []string{"bug", "bug"}}
```
**Expected:** `Apply(tasks, opts)` returns tasks with IDs `[1, 2]` (duplicate filter labels are checked redundantly but produce correct AND semantics — do NOT deduplicate opts.Labels)

### TestFilterLabelWithSpecialCharacters
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"bug-123"}},
    {ID: 2, Labels: []string{"bug/fix"}},
    {ID: 3, Labels: []string{"bug"}},
}
opts := Options{Labels: []string{"bug-123"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1` (exact string match including special characters)

### TestFilterLabelsOrderIndependent
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"frontend", "bug"}},
    {ID: 2, Labels: []string{"bug", "backend"}},
}
opts := Options{Labels: []string{"bug", "frontend"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1` (order of labels on task doesn't matter)

### TestFilterLabelsWithQuery
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Title: "Fix crash", Labels: []string{"bug"}},
    {ID: 2, Title: "Add feature", Labels: []string{"bug"}},
    {ID: 3, Title: "Fix crash", Labels: []string{"feature"}},
}
opts := Options{Query: "crash", Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1`

### TestFilterLabelCaseSensitivity
**Input:**
```go
tasks := []model.Task{
    {ID: 1, Labels: []string{"bug"}},
    {ID: 2, Labels: []string{"Bug"}},
}
opts := Options{Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1` (case-sensitive exact match — uppercase rejected)

### TestFilterAllFiltersCombined
**Input:**
```go
active := model.StatusActive
high := model.PriorityHigh
tasks := []model.Task{
    {ID: 1, Status: model.StatusActive, Priority: model.PriorityHigh, Title: "Fix crash", Labels: []string{"bug", "urgent"}},
    {ID: 2, Status: model.StatusActive, Priority: model.PriorityHigh, Title: "Fix crash", Labels: []string{"feature"}},
    {ID: 3, Status: model.StatusActive, Priority: model.PriorityLow, Title: "Fix crash", Labels: []string{"bug"}},
    {ID: 4, Status: model.StatusCompleted, Priority: model.PriorityHigh, Title: "Fix crash", Labels: []string{"bug"}},
}
opts := Options{Status: &active, Priority: &high, Query: "crash", Labels: []string{"bug"}}
```
**Expected:** `Apply(tasks, opts)` returns only task with ID `1`

## Edge Cases

1. **Empty Labels on Options** — Both `nil` and `[]string{}` mean no label filtering; `len(opts.Labels) == 0` check handles both
2. **Task with nil Labels** — `hasAllLabels` returns false for any non-empty required list
3. **Label comparison** — Direct string equality (`==`). Labels on `Task.Labels` are pre-normalized lowercase by the store phase. The filter does NOT normalize.
4. **AND semantics** — Multiple labels in `opts.Labels` means the task must have ALL of them, not just any one

## Integration Points

### Consumed by
- Phase cli: `list --label bug --label frontend` passes `[]string{"bug", "frontend"}` to `filter.Options.Labels`

### Depends on
- Phase model: `model.Task.Labels` field (`[]string`) must exist on the Task struct

### Exports
- `filter.Options.Labels` field (`[]string`) — set by cli phase when `--label` flags are provided
