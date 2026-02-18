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
3. Test naming conventions (`qa_{{phase}}_*`)
4. Test quality and maintainability

{{> common/reasoning-format.md}}

## Verdict

After your analysis, provide one of these verdicts:

- **approved** — Tests adequately cover the specification
- **gaps_found** — Tests are missing coverage (list specific gaps)
