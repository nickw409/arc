# Phase: integration

## Objective

Verify that all implemented phases work together correctly in the complete system.

## Files

### Create
- `[crate]/tests/integration_[PLAN_NAME].rs` — Integration tests for this plan

### Modify
- None (this phase only adds tests, no implementation changes)

## Types and Signatures

No new types. This phase tests existing types from previous phases.

## Test Cases

### Integration Test Categories

Write tests that verify cross-phase functionality:

1. **End-to-end workflows** — Complete user scenarios that touch multiple phases
2. **Data flow verification** — Data correctly passes between phase boundaries
3. **Error propagation** — Errors from early phases correctly surface through later phases
4. **State consistency** — System state remains consistent across phase interactions

### test_integration_[workflow_name]
**Setup:** Initialize components from phases [list phases]
**Input:** [Realistic end-to-end input]
**Expected:** [Final output after data flows through all phases]

### test_cross_phase_error_[scenario]
**Setup:** Initialize components from phases [list phases]
**Input:** [Input that causes error in phase N]
**Expected:** Error correctly propagates and is handled by phase M

## Edge Cases

1. **Phase boundary conditions** — Edge cases at the interface between phases
2. **Ordering dependencies** — Operations that must happen in specific order across phases
3. **Partial failures** — What happens when one phase fails mid-workflow
4. **Resource cleanup** — Resources are properly cleaned up across phase boundaries

## DO NOT

- [ ] Do NOT modify implementation code — only add tests
- [ ] Do NOT duplicate phase-specific unit tests — focus on cross-phase interactions
- [ ] Do NOT test internal implementation details — test public interfaces
- [ ] Do NOT skip error path testing — integration errors are critical

## Integration Points

### Phases to integrate
<!-- This will be filled based on the plan's phases -->
- All previous phases in this plan

### External systems
- List any external systems (DB, cache, APIs) involved in integration

---

## Checklist (for plan author)

Before sub-agents begin:

- [ ] Identified key workflows that span multiple phases
- [ ] Listed critical data flows between phases
- [ ] Documented error scenarios that cross phase boundaries
- [ ] Specified which crate(s) will contain integration tests
