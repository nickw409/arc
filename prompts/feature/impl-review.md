# Implementation Review - {{phase}}

You are reviewing the implementation for phase: **{{phase}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

### Test Results

Tests passing: {{state.tests_passing | default: "unknown"}} / {{state.tests_total | default: "unknown"}}

## Instructions

Review the implementation for:
1. All tests passing
2. Code follows specification
3. No test modifications (unless `allow_test_changes` was set)
4. Code quality and maintainability

{{> common/reasoning-format.md}}

## Verdict

After your analysis, provide one of these verdicts:

- **approved** — Implementation is correct and complete
- **concerns** — Implementation has issues (list specific concerns)
