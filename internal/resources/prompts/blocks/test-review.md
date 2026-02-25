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
1. Coverage of all test cases in the spec's **## Test Cases** section — every one must be implemented
2. Coverage of all edge cases in the spec's **## Edge Cases** section
3. Tests compile and fail correctly (no implementation exists yet)
4. Test quality: clear names, focused assertions, no testing multiple things in one test

## Response Format

Provide your analysis, then end with a verdict and memory section.

Before the verdict, write a memory section:

```
## Memory
[Key observations: what tests you checked, what was covered, what gaps were found.]
```

Then end with the verdict in this EXACT format (NOT inside a code block):

## Verdict

approved

The `## Verdict` header and verdict value MUST appear in your output — not inside a code block. The verdict value must be on its own line immediately after the header (blank lines between are ok). Valid verdicts:

- **approved** — Tests adequately cover the specification
- **gaps_found** — Tests are missing coverage (list specific gaps above the verdict)
