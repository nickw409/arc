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
- [ ] Wiring is tested — fix is reachable through the real entry point, not just tested in isolation
- [ ] Negative tests exist for adjacent inputs and related error paths
- [ ] Malformed/corrupt input variants are tested

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

The `## Verdict` header and verdict value MUST appear in your output — not inside a code block. The verdict value must be on its own line immediately after the header (blank lines between are ok). Valid verdicts:

- **approved** — Tests adequately cover the bug
- **gaps_found** — Must address gaps (list specific gaps above the verdict)
