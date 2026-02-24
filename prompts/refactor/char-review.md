# Characterization Test Review

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Characterization: {{phase_dir}}/characterization.md

## Your Task

Review the characterization tests to ensure they adequately capture current behavior.

## Review Checklist

- [ ] All public interfaces have tests
- [ ] Tests pass with current code
- [ ] Edge cases are covered
- [ ] Tests capture actual (not ideal) behavior
- [ ] Any known bugs are documented, not fixed

## Rules
- Ensure ALL interfaces are tested before refactoring begins
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

- **approved** — Characterization is complete
- **gaps_found** — Must address gaps (list specific gaps above the verdict)
