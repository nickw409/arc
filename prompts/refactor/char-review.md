# Characterization Test Review

Plan: {{plan_name}}
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

## Write Review

Create `{{phase_dir}}/char_review.md`:

```markdown
# Characterization Review: {{phase}}

## Coverage
- Interfaces tested: X/Y
- Edge cases: covered/missing

## Issues Found
- [ ] <missing interface>
- [ ] <missing edge case>

## Test Quality
- [ ] Tests are deterministic
- [ ] Tests are independent
- [ ] Tests capture behavior, not implementation

## Verdict
APPROVED - Characterization is complete
OR
GAPS_FOUND - Must address: <list>
```

## Rules
- Ensure ALL interfaces are tested before refactoring begins
- **Do NOT commit** - orchestrator handles commits

When done, exit.
