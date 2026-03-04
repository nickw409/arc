# Bug Fix Implementation

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Investigation: {{phase_dir}}/investigation.md

## Your Task

Implement the fix for the bug identified in the investigation.

## Steps

1. **Read the investigation** for root cause and recommended fix
2. **Read the regression tests** to understand expected behavior
3. **Implement the minimal fix** that makes tests pass
4. **Run tests** to verify the fix:
   ```bash
   {{scripts_dir}}/run-phase-tests.sh {{plan}} {{phase}}
   ```
5. **Document your changes** in `{{phase_dir}}/fix_reasoning.md`

## Fix Document

Create `{{phase_dir}}/fix_reasoning.md`:

```markdown
# Fix Reasoning: {{phase}}

## Root Cause (from investigation)
<summary>

## Fix Applied

### Change 1: <file:line>
- **Before**: <old code>
- **After**: <new code>
- **Why**: <explanation>

## Test Results
- All regression tests passing: Yes/No
- Any unexpected side effects: Yes/No

## Verification
<how you verified the fix is correct>
```

## Rules
- Fix ONLY the bug - no refactoring, no improvements
- Keep the change as small as possible
- Do NOT modify the regression tests
- **Do NOT commit** - orchestrator handles commits

Use `arc manage {{plan}} {{phase}} tests <passing> <total>` after running tests.

When done, write:

## Memory
[What fix was applied, which files were changed, test results. Future runs of this state will see this.]

## Verdict
done
