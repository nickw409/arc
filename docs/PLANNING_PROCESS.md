# Planning Process

## Overview

Planning is the most critical part of the orchestration workflow. A bad plan guarantees failure. The planning process ensures plans are thorough, unambiguous, and executable before any code is written.

## Process Flow

```
Human Request
     |
     v
+---------------------------------------------------------+
|  1. ANALYSIS                                            |
|                                                         |
|  Plan Agent:                                            |
|  - Investigates codebase                                |
|  - Identifies affected files                            |
|  - Determines work type (feature/bugfix/etc)            |
|  - Estimates scope                                      |
+---------------------------------------------------------+
     |
     v
+---------------------------------------------------------+
|  2. WORKFLOW SELECTION                                  |
|                                                         |
|  Plan Agent:                                            |
|  - Selects base workflow for work type                  |
|  - Identifies customizations needed                     |
|  - Generates workflow.yaml (extends base or custom)     |
+---------------------------------------------------------+
     |
     v
+---------------------------------------------------------+
|  3. PHASE DECOMPOSITION                                 |
|                                                         |
|  Plan Agent:                                            |
|  - Breaks work into phases                              |
|  - Ensures each phase is appropriately scoped           |
|  - Defines dependencies between phases                  |
|  - Writes plan.md for each phase                        |
+---------------------------------------------------------+
     |
     v
+---------------------------------------------------------+
|  4. ADVERSARIAL REVIEW                                  |
|                                                         |
|  Adversary Committee attacks plans:                     |
|  - Coverage: Are all cases handled?                     |
|  - Ambiguity: Could this be misinterpreted?             |
|  - Scope: Is this too big?                              |
|  - Consistency: Does this contradict itself?            |
|  - Executability: Can this actually be done?            |
|                                                         |
|  Loop until all adversaries satisfied or max iterations |
+---------------------------------------------------------+
     |
     v
+---------------------------------------------------------+
|  5. HUMAN APPROVAL                                      |
|                                                         |
|  Human reviews:                                         |
|  - Plan structure                                       |
|  - Workflow customizations                              |
|  - Adversary review results                             |
|  - Approves or requests changes                         |
+---------------------------------------------------------+
     |
     v
   Orchestrator Execution
```

## Plan Templates by Work Type

Each work type has a specific template with required sections.

### Feature Template

```markdown
---
type: feature
---

# Phase: [phase-name]

## Objective
[One sentence describing what this phase adds]

## Files

### Create
- `path/to/new_file.rs` -- [description]

### Modify
- `path/to/existing.rs` -- [what changes]

## Types and Signatures

```rust
// Complete, exact signatures. No pseudocode.
pub struct TypeName {
    pub field1: Type1,
    pub field2: Option<Type2>,
}

pub fn function_name<T: Bound>(arg: &T) -> Result<ReturnType, ErrorType>
```

## Error Types

```rust
#[derive(Debug, thiserror::Error)]
pub enum PhaseError {
    #[error("message: {0}")]
    VariantName(String),
}
```

## Dependencies

```toml
# Dependency additions
package_name = "1.2.3"
```

## DO NOT
- [ ] Do NOT [common mistake specific to this phase]
- [ ] Do NOT use unwrap()/expect()
- [ ] Do NOT modify files outside scope

## Test Cases

### test_name
**Input:**
```rust
let input = ConcreteValue { ... };
```
**Expected:** `function(&input)` returns `Ok(expected_value)`

## Edge Cases
1. **Empty input** -- [expected behavior]
2. **Null/None fields** -- [expected behavior]
3. **Boundary values** -- [expected behavior]

## Integration Points

### Consumed by
- Phase XX: [how used]

### Depends on
- Phase YY: [what's needed]
```

### Bug Fix Template

```markdown
---
type: bugfix
---

# Phase: [phase-name]

## Objective
[One sentence describing what bug this fixes]

## Current Behavior
[Describe the incorrect behavior with specifics]
- What happens now: [specific behavior]
- Why it's wrong: [explanation]
- How to reproduce: [steps]

## Correct Behavior
[Describe what should happen instead]
- Expected: [specific behavior]
- Why this is correct: [explanation]

## Root Cause Analysis
[If known, describe the underlying cause]
- Location: `file:line`
- Issue: [what's wrong with the code]

## Files

### Modify
- `path/to/buggy_file.rs` -- [what changes]

## The Fix

```rust
// Show the specific change needed
// Before:
incorrect_code();

// After:
correct_code();
```

## Regression Tests

### test_bug_does_not_regress
**Setup:** [how to create the bug condition]
**Input:** [specific input that triggered bug]
**Expected:** [correct behavior, not the bug]

### test_related_functionality_still_works
**Input:** [normal input]
**Expected:** [normal behavior unchanged]

## Verification
- [ ] Bug no longer reproduces with original steps
- [ ] Related functionality still works
- [ ] No new warnings/errors introduced

## DO NOT
- [ ] Do NOT change unrelated code
- [ ] Do NOT "improve" surrounding code
- [ ] Do NOT modify public API unless necessary
```

### Investigation Template

```markdown
---
type: investigation
---

# Phase: [phase-name]

## Objective
[One sentence describing what question this answers]

## Questions to Answer

1. [Specific question 1]
2. [Specific question 2]
3. [Specific question 3]

## Scope of Investigation

### Files to Examine
- `path/to/file1.rs` -- [what to look for]
- `path/to/file2.rs` -- [what to look for]

### Out of Scope
- [What NOT to investigate]

## Expected Deliverables

### findings.md
Must contain:
- Answer to each question with evidence
- Code references (file:line)
- Recommendations (if applicable)

### Format
```markdown
# Investigation Findings: [topic]

## Question 1: [question]
**Answer:** [answer]
**Evidence:** [code references, data]

## Question 2: [question]
...

## Recommendations
1. [recommendation]
2. [recommendation]
```

## DO NOT
- [ ] Do NOT modify any code
- [ ] Do NOT make changes "while you're in there"
- [ ] Do NOT spend time on tangential questions
```

### Refactor Template

```markdown
---
type: refactor
---

# Phase: [phase-name]

## Objective
[One sentence describing the structural change]

## Current Structure
[Describe how code is organized now]
- `file1.rs`: [contains what]
- `file2.rs`: [contains what]

## Target Structure
[Describe how code should be organized after]
- `new_file1.rs`: [will contain what]
- `new_file2.rs`: [will contain what]

## Invariants (MUST NOT CHANGE)

### Behavior Invariants
- [ ] [Specific behavior that must remain identical]
- [ ] [Another behavior]

### API Invariants
- [ ] [Public function signature that must not change]
- [ ] [Type that must remain compatible]

## Files

### Create
- `path/to/new_file.rs` -- [extracted from where]

### Modify
- `path/to/existing.rs` -- [what moves/changes]

### Delete
- `path/to/old_file.rs` -- [absorbed into where]

## Characterization Tests

Before refactoring, these tests capture current behavior:

### test_current_behavior_1
**Input:** [specific input]
**Current Output:** [what it does now - must match after refactor]

## Migration Steps

1. [First step]
2. [Second step]
3. [Third step]

## DO NOT
- [ ] Do NOT change behavior (this is refactor, not feature)
- [ ] Do NOT "improve" logic while moving it
- [ ] Do NOT change public APIs
```

### Performance Template

```markdown
---
type: performance
---

# Phase: [phase-name]

## Objective
[One sentence describing the performance goal]

## Current Performance
- Metric: [what's measured]
- Current value: [number with units]
- Target value: [number with units]
- Measurement method: [how to measure]

## Bottleneck Analysis
[Where is time/memory being spent?]
- `file:line` -- [what's slow and why]

## Proposed Optimization
[What change will improve performance?]
- Approach: [algorithm change, caching, parallelization, etc.]
- Expected improvement: [estimate]
- Tradeoffs: [memory vs speed, complexity, etc.]

## Files

### Modify
- `path/to/slow_file.rs` -- [what changes]

## Benchmarks

### Before Optimization
```rust
#[bench]
fn bench_current_implementation(b: &mut Bencher) {
    // Specific benchmark setup
}
```

### After Optimization
Same benchmark, should show improvement.

## Correctness Tests
[Performance optimization must not change behavior]

### test_output_unchanged
**Input:** [specific input]
**Expected:** [same output as before optimization]

## DO NOT
- [ ] Do NOT change behavior
- [ ] Do NOT optimize without measuring
- [ ] Do NOT sacrifice correctness for speed
```

## Workflow Generation

The Plan Agent generates or customizes workflows based on work type and specific needs.

### Base Workflow Selection

| Work Type | Base Workflow | When to Customize |
|-----------|--------------|-------------------|
| Feature | `workflows/feature.yaml` | Multi-package, unusual test setup |
| Bug Fix | `workflows/bugfix.yaml` | Requires investigation first |
| Investigation | `workflows/investigation.yaml` | Rarely needs customization |
| Refactor | `workflows/refactor.yaml` | Large-scale moves |
| Performance | `workflows/performance.yaml` | GPU/SIMD specific |

### Customization Examples

**Bug fix requiring server setup:**
```yaml
extends: bugfix

states:
  - name: start_test_server
    prompt: prompts/common/start-server.md
    insert_before: regression_tests
```

**Feature with cross-engine verification:**
```yaml
extends: feature

states:
  - name: cross_engine_verify
    prompt: prompts/custom/cross-engine.md
    params:
      engines: [cpu, gpu, wasm]
      tolerance: 0.01
    insert_after: impl
```

## Phase Decomposition Guidelines

### When to Split

- Phase affects >3 packages
- >10 files to modify
- >15 functions to implement
- Clear dependency boundaries exist
- Risk of partial completion

### How to Split

1. **By layer**: Types -> Implementation -> Integration
2. **By module**: Module A -> Module B -> Module C
3. **By feature**: Core -> Extensions -> Polish
4. **By dependency**: No deps -> Has deps -> Integration

### Phase Ordering

```
Phase 1: Foundation (no dependencies)
    |
    +---> Phase 2A: Feature branch A
    |         |
    +---> Phase 2B: Feature branch B
              |
              v
         Phase 3: Integration (depends on 2A, 2B)
              |
              v
         Phase 4: Polish/Tests
```

## Plan Quality Checklist

Before adversarial review, Plan Agent self-checks:

### Completeness
- [ ] Every affected file listed
- [ ] Every new type fully specified
- [ ] Every function has signature
- [ ] Every error variant defined
- [ ] Every test case has concrete input/output

### Clarity
- [ ] No pseudocode in signatures
- [ ] No "appropriate" or "suitable" without definition
- [ ] No references to "the usual way"
- [ ] File paths are absolute from project root

### Scope
- [ ] Phase can be completed in one session
- [ ] Dependencies on other phases are explicit
- [ ] DO NOT section covers likely mistakes

### Testability
- [ ] Tests can run in isolation
- [ ] No implicit dependencies on external state
- [ ] Edge cases are enumerated
