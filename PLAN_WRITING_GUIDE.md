# Plan Writing Guide

How to write phase plans that sub-agents can execute without making stupid decisions.

## The Problem

Sub-agents work in isolation. They see:
- The plan.md file
- The codebase
- Their mode (qa/impl/fix)

They do NOT see:
- The conversation where the plan was conceived
- Clarifications you discussed
- Your mental model of "how it should work"

**If ambiguous, sub-agents will guess. They will guess wrong.**

## Use the Template

Start every phase plan from: `$ARC_HOME/templates/plan-template.md`

The template enforces all required sections.

## Key Principles

### 1. Exact Signatures, Not Pseudocode

```rust
// BAD
pub fn serialize(data) -> Result<bytes, error>;

// GOOD
pub fn serialize_data<T: Serialize>(data: &T) -> Result<Vec<u8>, DataError> {
    // Uses bincode + lz4 compression with 4-byte size prefix
}
```

### 2. Explicit Paths, Not "The Crate"

```
// BAD
Add types to the server crate.

// GOOD
File: my-package/src/data.rs
Add to my-package/src/lib.rs: pub mod data;
```

### 3. DO NOT Sections Prevent Mistakes

```markdown
## DO NOT
- Do NOT put these types in my-project-shared (this phase is self-contained)
- Do NOT use serde_json (use bincode for binary serialization)
- Do NOT use unwrap() - all errors must propagate
```

### 4. Test Cases Need Concrete Inputs

```markdown
// BAD
Test that serialization works.

// GOOD
### test_serialize_roundtrip
Input: SimulationItem { item_id: 12345, std_cost: 99.99, ... }
Expected: deserialize(serialize(&item)) == item
```

### 5. Edge Cases Must Be Enumerated

```markdown
## Edge Cases
1. Empty collections: vec![] is valid, not an error
2. All optional fields None: valid SimulationItem
3. Negative values: std_cost can be negative (represents credit)
```

## Refinement Process

One phase at a time:

1. Orchestrator reads plan.md back to human
2. Human points out gaps
3. Iterate until "sub-agent proof"
4. Write to disk
5. Move to next phase (previous plan exits memory)

## Checklist

Before spawning sub-agents:

- [ ] Every signature is complete (generics, bounds, return types)
- [ ] Every file path is explicit
- [ ] Every error variant has a message format
- [ ] DO NOT section covers likely mistakes
- [ ] Test cases have concrete inputs and outputs
- [ ] Edge cases are enumerated
- [ ] Integration points are documented

If a sub-agent makes a wrong choice, the plan was ambiguous. Fix the plan, not just the code.
