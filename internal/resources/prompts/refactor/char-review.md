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

Before the verdict, write:

```
## Memory
[What interfaces you reviewed, coverage assessment, gaps found if any.]
```

Then the verdict (NOT inside a code block). The `## Verdict` header and verdict value MUST appear in your output. Valid verdicts:

- **approved** — Characterization is complete
- **gaps_found** — Must address gaps (list specific gaps above the verdict)
