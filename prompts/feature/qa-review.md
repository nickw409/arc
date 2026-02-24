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
