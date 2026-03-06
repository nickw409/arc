# Phase: [PHASE_NAME]

## Objective

[One sentence describing what this phase accomplishes]

## Spec

> This section becomes `spec.yaml`. It is required — phases with no spec are blocked from running.
> For each checkpoint, provide a name and a shell command that passes when the work is done.
> Simple fixes don't need checkpoints, but must have a description in the spec field.

```yaml
name: [PHASE_NAME]
complexity: simple|medium|complex
spec: |
  [Copy the Objective and Detailed Changes here as the agent's instructions]
checkpoints:
  - name: compiles
    description: Package builds without errors
    test: go build ./path/to/pkg/
  - name: [milestone-name]
    description: [What this verifies]
    test: [shell command that exits 0 on success]
  - name: tests-pass
    description: All tests pass
    test: go test ./path/to/pkg/
gate:
  assertions:
    - file_exists: path/to/new/file.go
    - grep: "func NewFunction"
```

## Files

### Create
- `path/to/new_file` — [brief description]

### Modify
- `path/to/existing_file` — [what changes]

## Prerequisites

Before making changes, verify:
1. [Thing to check — e.g., "read the X type definition to confirm field names"]
2. [Another prereq — or remove this section if none]

## Detailed Changes

### 1. [Change area] (`filename`)

[Describe what to add/modify and why]

```
// Complete, exact signatures. No pseudocode.

TypeName {
    field1: Type1
    field2: Type2
}

function_name(arg1: Type1, arg2: Type2) -> ReturnType
```

### 2. [Next change area] (`filename`)

[Description and code block]

## Test Cases

### test_name_1
**Input:** [concrete input values]
**Expected:** [exact expected output]

### test_name_2
**Input:** [invalid/edge input]
**Expected:** [specific error or empty result — state which one]

### test_edge_case_empty
**Input:** empty/zero values
**Expected:** [valid result | specific error] — state which one

## Edge Cases

1. **Empty input** — [valid or error?]
2. **Null/None fields** — [which fields can be null, behavior when null]
3. **Boundary values** — [max/min values, behavior at boundaries]
4. **Large inputs** — [any size limits or memory considerations]

## DO NOT

- Do NOT [common mistake 1 specific to this phase]
- Do NOT [common mistake 2]
- Do NOT ignore errors — handle or propagate all failure cases
- Do NOT modify files outside the scope listed above

---

## Checklist (for plan author)

Before marking ready for sub-agents:

- [ ] Every type has exact field definitions
- [ ] Every function has full signature with parameter and return types
- [ ] File paths are explicit (not "somewhere in the codebase")
- [ ] DO NOT section covers likely mistakes for this specific phase
- [ ] Test cases have concrete inputs and expected outputs
- [ ] Edge cases enumerated
- [ ] Verify compilation/build after changes
