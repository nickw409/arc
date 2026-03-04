# Refactoring Implementation

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Characterization: {{phase_dir}}/characterization.md

## Your Task

Perform the structural refactoring described in the plan while keeping all characterization tests passing.

## Steps

1. **Read the plan** for specific refactoring instructions
2. **Run characterization tests** to establish baseline:
   ```bash
   {{scripts_dir}}/run-phase-tests.sh {{plan}} {{phase}}
   ```
3. **Make structural changes** in small increments
4. **Run tests after each change** to catch regressions:
   ```bash
   {{scripts_dir}}/run-phase-tests.sh {{plan}} {{phase}}
   ```
5. **Document changes** in `{{phase_dir}}/refactor_log.md`

## Refactoring Rules

- NEVER change behavior - only structure
- Keep tests passing after EVERY change
- Make small, incremental changes
- If a test fails, revert and try a different approach

## Refactoring Log

Create `{{phase_dir}}/refactor_log.md`:

```markdown
# Refactoring Log: {{phase}}

## Changes Made

### Change 1: <description>
- **Files**: <list>
- **Type**: rename/extract/inline/move
- **Tests**: still passing

### Change 2: ...

## Test Status
- Before: X passing
- After: X passing (should be same)

## Structural Improvements
<what's better about the new structure>
```

## Rules
- Do NOT change behavior
- Do NOT add features
- Do NOT fix bugs (unless plan explicitly includes them)
- **Do NOT commit** - orchestrator handles commits

Use `arc manage {{plan}} {{phase}} tests <passing> <total>` after running tests.

When done, write:

## Memory
[What was refactored, changes made, test results before/after. Future runs of this state will see this.]

## Verdict
done
