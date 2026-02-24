# QA Review - {{phase}}

You are reviewing tests for phase: **{{phase}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

### Test Results

Tests passing: {{state.tests_passing | default: "unknown"}} / {{state.tests_total | default: "unknown"}}

## Instructions

Review the tests for:
1. Coverage of all specification requirements
2. Edge case coverage
3. Negative test coverage — invalid inputs, boundary violations, malformed data, error propagation, state violations
4. Test naming conventions match the project's existing style
5. Test quality and maintainability

### Wiring Test Checklist

For each integration point in the spec, verify tests exist that:
- [ ] Call the top-level entry point and check it reaches inner functions (not just testing inner functions in isolation)
- [ ] Pass a value through the full call chain and assert it arrives correctly
- [ ] Return errors from inner functions and verify the outer function propagates them
- [ ] Trigger side effects (file writes, state updates) through the real entry point, not by calling the helper directly

If wiring tests are missing, verdict MUST be **gaps_found**. A test suite that only tests helpers in isolation but never tests that they're called is incomplete.

### Negative Test Checklist

For each public function/method, verify tests exist for:
- [ ] Invalid/nil/empty inputs
- [ ] Boundary violations (off-by-one, overflow, underflow)
- [ ] Malformed or corrupt data
- [ ] Error propagation from dependencies
- [ ] Invalid state (wrong call order, closed resources)

If negative tests are missing, verdict MUST be **gaps_found** with specific gaps listed.

{{> common/reasoning-format.md}}

## Verdict

After your analysis, provide one of these verdicts:

- **approved** — Tests adequately cover the specification
- **gaps_found** — Tests are missing coverage (list specific gaps)
