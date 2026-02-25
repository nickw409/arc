# Regression Test Review

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Investigation: {{phase_dir}}/investigation.md

## Your Task

Review the regression tests to ensure they:
1. Actually test the bug described in the investigation
2. Will fail before fix and pass after fix
3. Cover edge cases related to the bug

## Review Checklist

- [ ] Test reproduces the exact bug from investigation
- [ ] Test name clearly describes what it's testing
- [ ] Assertions will pass when bug is fixed (not overly strict)
- [ ] Edge cases are covered (boundary values, empty inputs, etc.)
- [ ] Tests don't rely on implementation details

## Rules
- Be skeptical - ensure tests actually catch the bug
- **Do NOT commit** - orchestrator handles commits

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

Before the verdict, write:

```
## Memory
[What tests you reviewed, what checklist items passed/failed, gaps found.]
```

Then the verdict (NOT inside a code block). The `## Verdict` header and verdict value MUST appear in your output. Valid verdicts:

- **approved** — Tests adequately cover the bug
- **gaps_found** — Must address gaps (list specific gaps above the verdict)
