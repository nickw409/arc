# Bug Fix Review

Plan: {{plan}}
Phase: {{phase}}
Investigation: {{phase_dir}}/investigation.md
Fix reasoning: {{phase_dir}}/fix_reasoning.md

## Your Task

Review the bug fix to ensure it:
1. Actually fixes the root cause (not just symptoms)
2. Is minimal and doesn't introduce other changes
3. Doesn't break existing functionality

## Review Checklist

- [ ] Fix addresses the root cause from investigation
- [ ] Change is minimal - no unrelated modifications
- [ ] All regression tests pass
- [ ] No new warnings or errors introduced
- [ ] Fix reasoning is sound

## Rules
- Be thorough - bugs can recur if fix is incomplete
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

concerns
```

The `## Verdict` header and verdict value MUST appear in your output — not inside a code block. The verdict value must be on its own line immediately after the header (blank lines between are ok). Valid verdicts:

- **approved** — Fix is correct and minimal
- **concerns** — Address before proceeding (list specific concerns above the verdict)
