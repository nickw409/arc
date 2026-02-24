# Phase: model

## Objective

Add label/tag support to the Task data model with validation and normalization helpers.

## Files

### Modify
- `internal/model/task.go` — Add `Labels` field to Task struct, add `NormalizeLabel` and `ValidateLabel` package-level functions

### Create
- `testbed/internal/model/label_test.go` — Unit tests for label helpers and Task.Labels serialization (absolute path from project root)

## Types and Signatures

```go
// Add `Labels` field to the existing Task struct.
// Add it as the final field, after all existing fields (ID, Description, Status, Priority, CreatedAt, CompletedAt).
// Do NOT remove or reorder any existing fields.
type Task struct {
    // ... existing fields unchanged ...
    Labels []string `json:"labels,omitempty"`
}

// NormalizeLabel lowercases and trims whitespace from a label string.
// Uses strings.ToLower for ASCII lowercasing (not locale-aware).
func NormalizeLabel(s string) string {
    return strings.ToLower(strings.TrimSpace(s))
}

// ValidateLabel returns an error if the label is empty after normalization.
// Returns exactly: fmt.Errorf("label cannot be empty")
func ValidateLabel(s string) error {
    if NormalizeLabel(s) == "" {
        return fmt.Errorf("label cannot be empty")
    }
    return nil
}
```

**Contract for downstream phases:**
- `NormalizeLabel` and `ValidateLabel` are **package-level functions** — they are declared at package scope in `task.go`, not as methods on any struct type. They have no receiver. Call as `model.NormalizeLabel(s)` from other packages, or `NormalizeLabel(s)` within the `model` package.
- The store phase is the **sole enforcer of normalized persistence**: it always calls `ValidateLabel` then `NormalizeLabel` before persisting to disk, regardless of what the caller has already done. CLI callers pass raw user input to `Store.AddLabel`; they do NOT normalize first.
- The CLI phase calls `model.ValidateLabel(s)` for early input validation during task creation with the `add -l` flag (to give errors before creating the task). The store will re-validate and normalize independently.
- The `Task.Labels` field is a passive `[]string` container. It does NOT auto-normalize or auto-deduplicate. In-memory Task instances may contain denormalized values (uppercase, extra whitespace) and duplicates. The store phase is responsible for normalizing and deduplicating labels before writing them to the JSON file on disk. Once a task is reloaded from disk, `Task.Labels` will contain only normalized (lowercase, trimmed), unique strings because the store enforced this when saving.
- The filter phase compares labels using direct string equality. When filtering tasks loaded from the store, labels are already normalized. When filtering in-memory tasks that haven't been saved yet, labels may not be normalized.
- `Labels: nil` is the zero value. A task with no labels has `Labels: nil` (not `[]string{}`).

## Error Types

```go
// ValidateLabel returns exactly this error for invalid labels:
fmt.Errorf("label cannot be empty")
// This is the ONLY validation rule — any non-empty-after-trim string is valid.
// No length limits, no character restrictions, no format rules.
```

## Test Patterns

All tests use the standard `testing` package. Assertion conventions:

- **Slice equality:** Use `reflect.DeepEqual(got, want)` to compare `[]string` values. Import `reflect`.
- **Error assertions:** Use `err.Error() != "label cannot be empty"` within an `if` statement to detect error message mismatches, triggering `t.Errorf` when the condition is true (i.e., when the error message does not match the expected value). Never use `errors.Is` (these are `fmt.Errorf` values, not sentinel errors).
- **JSON key presence:** Marshal to `[]byte`, then use `bytes.Contains(data, []byte(`"`labels`"`))` to check if a key exists. Import `bytes` and `encoding/json`.
- **JSON key absence:** `!bytes.Contains(data, []byte(`"`labels`"`))`.
- **JSON round-trip:** Marshal a `Task`, then `json.Unmarshal` into a new `Task`, then `reflect.DeepEqual` on the `Labels` field.

Tests must import `testing` at the file level. Since the test cases below use JSON marshaling, reflection, and byte operations, the test file must also import `encoding/json`, `reflect`, and `bytes` at the file level. All imports are declared in the standard Go import block at the top of the file.

All test cases below should be implemented as individual `func TestXxx(t *testing.T)` functions (one function per test case). Do NOT use table-driven tests.

For assertion failures, use this pattern:
```go
if !reflect.DeepEqual(got, want) {
    t.Errorf("got %v, want %v", got, want)
}
```

For error message checks (when error is expected), use this pattern:
```go
if err == nil {
    t.Errorf("expected error, got nil")
} else if err.Error() != "label cannot be empty" {
    t.Errorf("got error %q, want %q", err.Error(), "label cannot be empty")
}
```

For nil error checks, use:
```go
if err != nil {
    t.Errorf("unexpected error: %v", err)
}
```

## Dependencies

None — production code uses only the standard library (`strings`, `fmt`). Tests additionally use `encoding/json`, `reflect`, `bytes`.

## DO NOT

- [ ] Do NOT modify any files outside `internal/model/`
- [ ] Do NOT add label-related methods to the Store (that is a later phase)
- [ ] Do NOT add CLI flags or commands
- [ ] Do NOT change existing field names or JSON tags on the Task struct
- [ ] Do NOT add an external dependency for string manipulation
- [ ] Do NOT use `golang.org/x/text/cases` — ASCII lowercasing via `strings.ToLower` is sufficient

## Test Cases

All test names use Go's `TestXxx` convention.

### TestNormalizeLabelLowercase
**Input:** `NormalizeLabel("URGENT")`
**Expected:** Returns `"urgent"`

### TestNormalizeLabelTrim
**Input:** `NormalizeLabel("  work  ")`
**Expected:** Returns `"work"`

### TestNormalizeLabelCombined
**Input:** `NormalizeLabel("  BUG fix  ")`
**Expected:** Returns `"bug fix"`

### TestNormalizeLabelEmpty
**Input:** `NormalizeLabel("")`
**Expected:** Returns `""`

### TestNormalizeLabelAlreadyNormalized
**Input:** `NormalizeLabel("bug")`
**Expected:** Returns `"bug"`

### TestNormalizeLabelUnicode
**Input:** `NormalizeLabel("ÜBER")`
**Expected:** Returns `"über"`

### TestNormalizeLabelUnicodeWhitespace
**Input:** `NormalizeLabel("\t\n")`
**Expected:** Returns `""`

### TestValidateLabelValid
**Input:** `ValidateLabel("bug")`
**Expected:** Returns `nil`

### TestValidateLabelEmpty
**Input:** `ValidateLabel("")`
**Expected:** Returns error with message exactly `"label cannot be empty"`

### TestValidateLabelWhitespaceOnly
**Input:** `ValidateLabel("   ")`
**Expected:** Returns error with message exactly `"label cannot be empty"`

### TestValidateLabelUnicodeWhitespace
**Input:** `ValidateLabel("\t\n")`
**Expected:** Returns error with message exactly `"label cannot be empty"`

### TestValidateLabelSpecialChars
**Input:** `ValidateLabel("bug/fix")`
**Expected:** Returns `nil` (any non-empty-after-trim string is valid)

### TestValidateLabelUnicode
**Input:** `ValidateLabel("ÜBER")`
**Expected:** Returns `nil`

### TestValidateLabelSingleChar
**Input:** `ValidateLabel("x")`
**Expected:** Returns `nil`

### TestValidateLabelSpecialCharsOnly
**Input:** `ValidateLabel("@#$")`
**Expected:** Returns `nil`

### TestNormalizeLabelTabsOnly
**Input:** `NormalizeLabel("\t\t")`
**Expected:** Returns `""`

### TestNormalizeLabelMultiWordUnicode
**Input:** `NormalizeLabel("Über DÜSSELDORF")`
**Expected:** Returns `"über düsseldorf"`

### TestNormalizeLabelMultipleInternalSpaces
**Input:** `NormalizeLabel("bug    fix")`
**Expected:** Returns `"bug    fix"` (internal whitespace is preserved as-is, only leading/trailing trimmed)

### TestNormalizeLabelSingleChar
**Input:** `NormalizeLabel("A")`
**Expected:** Returns `"a"`

### TestNormalizeLabelEmoji
**Input:** `NormalizeLabel("🐛BUG🐛")`
**Expected:** Returns `"🐛bug🐛"` (emoji preserved, ASCII uppercased letters lowercased)

### TestNormalizeLabelNonBreakingSpace
**Input:** `NormalizeLabel("\u00A0bug\u00A0")`
**Expected:** Returns `"bug"` (non-breaking space U+00A0 is trimmed by `strings.TrimSpace`)

### TestTaskLabelsNotAutoNormalized
**Input:** Create `Task{Labels: []string{"URGENT", "  spaces  "}}`, marshal to JSON, unmarshal back
**Expected:** Labels are `[]string{"URGENT", "  spaces  "}` — the model layer does NOT auto-normalize. Normalization is opt-in via helper functions and enforced by the store phase.

### TestTaskLabelsJsonRoundtrip
**Input:** Marshal a Task with `Labels: []string{"bug", "frontend"}` to JSON, then unmarshal into a new `Task`
**Expected:** `reflect.DeepEqual(got.Labels, []string{"bug", "frontend"})` is true

### TestTaskLabelsJsonNull
**Input:** Unmarshal JSON containing `"labels": null` into a `Task`
**Expected:** `task.Labels` is `nil`

### TestTaskLabelsJsonEmptyArray
**Input:** Unmarshal JSON containing `"labels": []` into a `Task`
**Expected:** `task.Labels` is `[]string{}` (not nil)

### TestTaskLabelsOmitemptyNil
**Input:** Marshal a Task with `Labels: nil` to JSON
**Expected:** `!bytes.Contains(data, []byte("labels"))` — the key is absent from the JSON

### TestTaskLabelsEmptySliceJson
**Input:** Marshal a Task with `Labels: []string{}` to JSON
**Expected:** `bytes.Contains(data, []byte(`"`labels`": []`))` — empty array is serialized, unlike nil

### TestTaskLabelsPreservesOrder
**Input:** Create Task with `Labels: []string{"frontend", "bug", "urgent"}`, marshal then unmarshal into a new `Task`
**Expected:** `reflect.DeepEqual(got.Labels, []string{"frontend", "bug", "urgent"})` is true

### TestTaskLabelsDuplicate
**Input:** Marshal a Task with `Labels: []string{"bug", "bug"}` to JSON, then unmarshal into a new `Task`
**Expected:** `reflect.DeepEqual(got.Labels, []string{"bug", "bug"})` is true (model layer does not deduplicate — that's the store's job)

### TestTaskLabelsWithEmptyString
**Input:** Create Task with `Labels: []string{"bug", "", "frontend"}`, marshal to JSON, unmarshal into a new `Task`
**Expected:** `reflect.DeepEqual(got.Labels, []string{"bug", "", "frontend"})` is true (model layer passively allows empty strings — store phase will validate and reject)

### TestTaskLabelsMixedNormalization
**Input:** Create Task with `Labels: []string{"bug", "URGENT", "  spaces  "}`, marshal to JSON, unmarshal into a new `Task`
**Expected:** `reflect.DeepEqual(got.Labels, []string{"bug", "URGENT", "  spaces  "})` is true (model layer preserves mixed normalized/denormalized values)

### TestNormalizeLabelVeryLongString
**Input:** `NormalizeLabel(strings.Repeat("A", 10000))`
**Expected:** Returns `strings.Repeat("a", 10000)` (handles very long input without error)

### TestValidateLabelVeryLongString
**Input:** `ValidateLabel(strings.Repeat("x", 10000))`
**Expected:** Returns `nil` (no length limit enforced at model layer)

## Edge Cases

1. **Nil vs empty slice** — `Labels: nil` omits from JSON (`omitempty`); `Labels: []string{}` serializes as `"labels": []`. Use `nil` as the zero value for tasks with no labels.
2. **Simple Unicode lowercasing** — `strings.ToLower` provides Unicode-aware lowercasing for common Latin/European characters (ü, é, ö, etc.) but not locale-specific edge cases (e.g., Turkish İ→i). This project uses `strings.ToLower` without locale support.
3. **Labels with internal whitespace** — `"bug fix"` is a valid single label after trim; internal spaces are preserved.
4. **Validation is minimal** — Only check: non-empty after normalization. No character restrictions, no length limits.

## Integration Points

### Consumed by
- Phase store: Calls `model.ValidateLabel(s)` then `model.NormalizeLabel(s)` before storing. Manipulates `task.Labels` slice directly (append/remove).
- Phase filter: Reads `task.Labels` and compares using direct string equality (labels are pre-normalized).
- Phase cli: Calls `model.ValidateLabel(s)` for early input validation during task creation with the `add -l` flag.

### Depends on
- Nothing — this is the first phase

### Exports
- `model.Task.Labels` field (`[]string`) — used by all subsequent phases
- `model.NormalizeLabel(s string) string` — package-level function, called by store and cli phases
- `model.ValidateLabel(s string) error` — package-level function, called by store and cli phases
