# Phase: [PHASE_NAME]

## Objective

[One sentence describing what this phase accomplishes]

## Files

### Create
- `path/to/new_file.rs` — [brief description]

### Modify
- `path/to/existing.rs` — [what changes]

## Types and Signatures

```
// Complete, exact signatures. No pseudocode.

/// [Doc comment explaining purpose]
pub struct TypeName {
    pub field1: Type1,
    pub field2: Option<Type2>,
}

/// [Doc comment]
pub fn function_name<T: Bound>(arg: &T) -> Result<ReturnType, ErrorType> {
    // Implementation notes if non-obvious
}
```

## Error Types

```
#[derive(Debug, thiserror::Error)]
pub enum PhaseError {
    #[error("Specific message with context: {0}")]
    VariantName(String),
    
    #[error("Another error: expected {expected}, got {actual}")]
    AnotherVariant { expected: usize, actual: usize },
}
```

## Dependencies

```toml
# Add to [crate]/Cargo.toml [dependencies]:
crate_name = "1.2.3"
another = { version = "2.0", features = ["feature1"] }
```

## DO NOT

- [ ] Do NOT [common mistake 1]
- [ ] Do NOT [common mistake 2]
- [ ] Do NOT use unwrap()/expect() — propagate errors via Result
- [ ] Do NOT modify files outside the scope listed above

## Test Cases

### test_name_1
**Input:**
```
let input = SomeStruct { field: value };
```
**Expected:** `function(&input)` returns `Ok(expected_value)`

### test_name_2
**Input:**
```
let invalid = SomeStruct { field: bad_value };
```
**Expected:** `function(&invalid)` returns `Err(PhaseError::VariantName(_))`

### test_edge_case_empty
**Input:** Empty collection `vec![]`
**Expected:** [Valid empty result | Specific error] — state which one

## Edge Cases

1. **Empty collections** — [valid or error?]
2. **Null/None fields** — [which fields can be None, behavior when None]
3. **Boundary values** — [max/min values, behavior at boundaries]
4. **Unicode** — [if strings involved, unicode handling]
5. **Large inputs** — [any size limits, memory considerations]

## Integration Points

### Consumed by
- Phase XX: [how it uses this phase's output]
- Crate YY: [external consumer]

### Depends on
- Phase ZZ: [what this phase needs from previous phases]
- External: [external dependencies like DB, files]

### Exports
List all `pub` items that other phases/crates will use:
- `TypeName` — used by [consumer]
- `function_name` — called by [consumer]
- `PhaseError` — matched by [consumer]

---

## Checklist (for plan author)

Before marking ready for sub-agents:

- [ ] Every struct/enum has exact field definitions with types
- [ ] Every function has full signature with generics and bounds
- [ ] Every error variant has specific message format
- [ ] File paths are explicit (not "somewhere in the crate")
- [ ] Cargo.toml changes listed with exact versions
- [ ] DO NOT section covers likely mistakes
- [ ] Test cases have concrete inputs and expected outputs
- [ ] Edge cases enumerated
- [ ] Integration points documented
