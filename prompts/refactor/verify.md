# Refactoring Verification

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Refactoring log: {{phase_dir}}/refactor_log.md

## Your Task

Verify that the refactoring:
1. Did not change behavior (all tests pass)
2. Achieved the structural goals from the plan
3. Did not introduce technical debt

## Verification Steps

1. **Run ALL characterization tests** - they must all pass:
   ```bash
   {{scripts_dir}}/run-phase-tests.sh {{plan}} {{phase}}
   ```
2. **Run full test suite** - no regressions
3. **Review changes** against plan goals
4. **Check for code smells** introduced by refactoring

## Write Verification Report

Create `{{phase_dir}}/verification.md`:

```markdown
# Refactoring Verification: {{phase}}

## Test Results
- Characterization tests: X/X passing
- Full test suite: X/X passing
- New failures: None/<list>

## Plan Goals Achieved
- [ ] <goal from plan> - achieved/not achieved

## Code Quality Check
- [ ] No duplicate code introduced
- [ ] No dead code left behind
- [ ] Names are clear and consistent
- [ ] No increase in complexity

## Concerns
- [ ] <concern>

## Verdict
APPROVED - Refactoring is complete and correct
OR
CONCERNS - Address before proceeding: <list>
```

## Rules
- ALL characterization tests MUST pass
- No behavior changes allowed
- **Do NOT commit** - orchestrator handles commits

When done, output a summary of the verification results and exit.
