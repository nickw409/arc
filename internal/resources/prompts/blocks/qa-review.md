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

## Response Format

Provide your analysis, then you MUST end your response with a verdict section in this EXACT format:

```
## Verdict

approved
```

OR

```
## Verdict

gaps_found
```

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
