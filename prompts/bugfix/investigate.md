# Bug Investigation

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}

## Your Task

Investigate the bug described in the plan to understand:
1. What is the expected behavior?
2. What is the actual behavior?
3. What is the root cause?

## Steps

1. **Read the plan** to understand the bug report
2. **Reproduce the bug** if possible (check test cases in plan)
3. **Trace the code path** from input to incorrect output
4. **Identify the root cause** - the specific code that's wrong
5. **Document your findings** in `{{phase_dir}}/investigation.md`

## Investigation Document

Create `{{phase_dir}}/investigation.md`:

```markdown
# Bug Investigation: {{phase}}

## Bug Summary
<one sentence description>

## Expected Behavior
<what should happen>

## Actual Behavior
<what currently happens>

## Reproduction Steps
1. <step>
2. <step>

## Root Cause Analysis

### Code Path
<trace from input to bug>

### Root Cause
<specific file:line and why it's wrong>

### Evidence
<test output, logs, values that prove this>

## Recommended Fix
<high-level approach>
```

## Rules
- Do NOT fix the bug yet - investigation only
- Do NOT modify any source code
- Focus on understanding, not solving
- **Do NOT commit** - orchestrator handles commits

When done, exit.
