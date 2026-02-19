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

## Write Review

Create `{{phase_dir}}/fix_review.md`:

```markdown
# Fix Review: {{phase}}

## Fix Summary
<what was changed>

## Root Cause Addressed?
Yes/No - <explanation>

## Concerns
- [ ] <concern>

## Test Results
- Regression tests: X/Y passing
- Other tests affected: None/List

## Verdict
APPROVED - Fix is correct and minimal
OR
CONCERNS - Address before proceeding: <list>
```

## Rules
- Be thorough - bugs can recur if fix is incomplete
- **Do NOT commit** - orchestrator handles commits

When done, exit.
