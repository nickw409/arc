# Regression Test Review

Plan: {{plan_name}}
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

## Write Review

Create `{{phase_dir}}/test_review.md`:

```markdown
# Test Review: {{phase}}

## Coverage
- Bug reproduction: Yes/No
- Edge cases: X/Y covered

## Issues Found
- [ ] <issue>

## Verdict
APPROVED - Tests adequately cover the bug
OR
GAPS_FOUND - Must address: <list>
```

## Rules
- Be skeptical - ensure tests actually catch the bug
- **Do NOT commit** - orchestrator handles commits

When done, exit.
