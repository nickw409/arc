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
2. Code follows the specification
3. No test files were modified
4. Code quality and maintainability

## Response Format

Provide your analysis, then end with a verdict and memory section.

Before the verdict, write a memory section:

```
## Memory
[Key observations: what you checked, what passed, what concerns were found.]
```

Then end with the verdict in this EXACT format (NOT inside a code block):

## Verdict

approved

The `## Verdict` header and verdict value MUST appear in your output — not inside a code block. The verdict value must be on its own line immediately after the header (blank lines between are ok). Valid verdicts:

- **approved** — Implementation is correct and complete
- **concerns** — Implementation has issues (list specific concerns above the verdict)
